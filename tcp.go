// Package tcp implements a go-micro.Server
package tcp

import (
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/unistack-org/micro/v3/broker"
	"github.com/unistack-org/micro/v3/codec"
	jsonrpc "github.com/unistack-org/micro/v3/codec/jsonrpc"
	protorpc "github.com/unistack-org/micro/v3/codec/protorpc"
	"github.com/unistack-org/micro/v3/logger"
	"github.com/unistack-org/micro/v3/registry"
	"github.com/unistack-org/micro/v3/server"
	"golang.org/x/net/netutil"
)

var (
	defaultCodecs = map[string]codec.NewCodec{
		"application/json":         jsonrpc.NewCodec,
		"application/json-rpc":     jsonrpc.NewCodec,
		"application/protobuf":     protorpc.NewCodec,
		"application/proto-rpc":    protorpc.NewCodec,
		"application/octet-stream": protorpc.NewCodec,
	}
)

type tcpServer struct {
	sync.RWMutex
	opts         server.Options
	hd           server.Handler
	exit         chan chan error
	registerOnce sync.Once
	subscribers  map[*tcpSubscriber][]broker.Subscriber
	// used for first registration
	registered bool
}

func (h *tcpServer) newCodec(contentType string) (codec.NewCodec, error) {
	if cf, ok := h.opts.Codecs[contentType]; ok {
		return cf, nil
	}
	if cf, ok := defaultCodecs[contentType]; ok {
		return cf, nil
	}
	return nil, fmt.Errorf("Unsupported Content-Type: %s", contentType)
}

func (h *tcpServer) Options() server.Options {
	h.RLock()
	defer h.RUnlock()
	return h.opts
}

func (h *tcpServer) Init(opts ...server.Option) error {
	h.Lock()
	for _, o := range opts {
		o(&h.opts)
	}
	h.Unlock()
	return nil
}

func (h *tcpServer) Handle(handler server.Handler) error {
	h.Lock()
	h.hd = handler
	h.Unlock()
	return nil
}

func (h *tcpServer) NewHandler(handler interface{}, opts ...server.HandlerOption) server.Handler {
	options := server.HandlerOptions{
		Metadata: make(map[string]map[string]string),
	}

	for _, o := range opts {
		o(&options)
	}

	var eps []*registry.Endpoint

	if !options.Internal {
		for name, metadata := range options.Metadata {
			eps = append(eps, &registry.Endpoint{
				Name:     name,
				Metadata: metadata,
			})
		}
	}

	th := &tcpHandler{
		eps:  eps,
		hd:   handler,
		opts: options,
	}

	if size, ok := h.opts.Context.Value(maxMsgSizeKey{}).(int); ok && size > 0 {
		th.maxMsgSize = size
	}

	return th
}

func (h *tcpServer) NewSubscriber(topic string, handler interface{}, opts ...server.SubscriberOption) server.Subscriber {
	return newSubscriber(topic, handler, opts...)
}

func (h *tcpServer) Subscribe(sb server.Subscriber) error {
	sub, ok := sb.(*tcpSubscriber)
	if !ok {
		return fmt.Errorf("invalid subscriber: expected *tcpSubscriber")
	}
	if len(sub.handlers) == 0 {
		return fmt.Errorf("invalid subscriber: no handler functions")
	}

	if err := validateSubscriber(sb); err != nil {
		return err
	}

	h.Lock()
	defer h.Unlock()
	_, ok = h.subscribers[sub]
	if ok {
		return fmt.Errorf("subscriber %v already exists", h)
	}
	h.subscribers[sub] = nil
	return nil
}

func (h *tcpServer) Register() error {
	h.Lock()
	opts := h.opts
	eps := h.hd.Endpoints()
	h.Unlock()

	service := serviceDef(opts)
	service.Endpoints = eps

	h.Lock()
	var subscriberList []*tcpSubscriber
	for e := range h.subscribers {
		// Only advertise non internal subscribers
		if !e.Options().Internal {
			subscriberList = append(subscriberList, e)
		}
	}
	sort.Slice(subscriberList, func(i, j int) bool {
		return subscriberList[i].topic > subscriberList[j].topic
	})
	for _, e := range subscriberList {
		service.Endpoints = append(service.Endpoints, e.Endpoints()...)
	}
	h.Unlock()

	rOpts := []registry.RegisterOption{
		registry.RegisterTTL(opts.RegisterTTL),
	}

	h.registerOnce.Do(func() {
		logger.Infof("Registering node: %s", opts.Name+"-"+opts.Id)
	})

	if err := opts.Registry.Register(service, rOpts...); err != nil {
		return err
	}

	h.Lock()
	defer h.Unlock()

	if h.registered {
		return nil
	}
	h.registered = true

	for sb := range h.subscribers {
		handler := h.createSubHandler(sb, opts)
		var subOpts []broker.SubscribeOption
		if queue := sb.Options().Queue; len(queue) > 0 {
			subOpts = append(subOpts, broker.Queue(queue))
		}

		if !sb.Options().AutoAck {
			subOpts = append(subOpts, broker.DisableAutoAck())
		}

		sub, err := opts.Broker.Subscribe(sb.Topic(), handler, subOpts...)
		if err != nil {
			return err
		}
		h.subscribers[sb] = []broker.Subscriber{sub}
	}
	return nil
}

func (h *tcpServer) Deregister() error {
	h.Lock()
	opts := h.opts
	h.Unlock()

	logger.Infof("Deregistering node: %s", opts.Name+"-"+opts.Id)

	service := serviceDef(opts)
	if err := opts.Registry.Deregister(service); err != nil {
		return err
	}

	h.Lock()
	if !h.registered {
		h.Unlock()
		return nil
	}
	h.registered = false

	for sb, subs := range h.subscribers {
		for _, sub := range subs {
			logger.Infof("Unsubscribing from topic: %s", sub.Topic())
			sub.Unsubscribe()
		}
		h.subscribers[sb] = nil
	}
	h.Unlock()
	return nil
}

func (h *tcpServer) getListener() net.Listener {
	if h.opts.Context == nil {
		return nil
	}

	l, ok := h.opts.Context.Value(netListener{}).(net.Listener)
	if !ok || l == nil {
		return nil
	}

	return l
}

func (h *tcpServer) Start() error {
	h.Lock()
	opts := h.opts
	hd := h.hd.Handler()
	h.Unlock()

	var err error
	var ts net.Listener

	if l := h.getListener(); l != nil {
		ts = l
	} else {
		// check the tls config for secure connect
		if tc := opts.TLSConfig; tc != nil {
			ts, err = tls.Listen("tcp", opts.Address, tc)
			// otherwise just plain tcp listener
		} else {
			ts, err = net.Listen("tcp", opts.Address)
		}
		if err != nil {
			return err
		}

		if opts.Context != nil {
			if c, ok := opts.Context.Value(maxConnKey{}).(int); ok && c > 0 {
				ts = netutil.LimitListener(ts, c)
			}
		}
	}

	logger.Infof("Listening on %s", ts.Addr().String())

	h.Lock()
	h.opts.Address = ts.Addr().String()
	h.Unlock()

	if err = opts.Broker.Connect(); err != nil {
		return err
	}

	// register
	if err = h.Register(); err != nil {
		return err
	}

	handle, ok := hd.(Handler)
	if !ok {
		return fmt.Errorf("invalid handler %T", hd)
	}
	go h.serve(ts, handle)

	go func() {
		t := new(time.Ticker)

		// only process if it exists
		if opts.RegisterInterval > time.Duration(0) {
			// new ticker
			t = time.NewTicker(opts.RegisterInterval)
		}

		// return error chan
		var ch chan error

	Loop:
		for {
			select {
			// register self on interval
			case <-t.C:
				if err := h.Register(); err != nil {
					logger.Error("Server register error: ", err)
				}
			// wait for exit
			case ch = <-h.exit:
				break Loop
			}
		}

		ch <- ts.Close()

		// deregister
		h.Deregister()

		opts.Broker.Disconnect()
	}()

	return nil
}

func (h *tcpServer) Stop() error {
	ch := make(chan error)
	h.exit <- ch
	return <-ch
}

func (h *tcpServer) String() string {
	return "tcp"
}

func (s *tcpServer) serve(ln net.Listener, h Handler) {
	for {
		c, err := ln.Accept()
		if err != nil {
			logger.Errorf("tcp: accept err: %v", err)
			continue
		}
		go h.Serve(c)
	}
}

func newServer(opts ...server.Option) server.Server {
	return &tcpServer{
		opts:        newOptions(opts...),
		exit:        make(chan chan error),
		subscribers: make(map[*tcpSubscriber][]broker.Subscriber),
	}
}

func NewServer(opts ...server.Option) server.Server {
	return newServer(opts...)
}
