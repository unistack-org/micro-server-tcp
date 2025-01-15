// Package tcp implements a go-micro.Server
package tcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.unistack.org/micro/v3/broker"
	"go.unistack.org/micro/v3/codec"
	"go.unistack.org/micro/v3/logger"
	"go.unistack.org/micro/v3/register"
	"go.unistack.org/micro/v3/server"
	msync "go.unistack.org/micro/v3/sync"
	"golang.org/x/net/netutil"
)

var _ server.Server = (*Server)(nil)

type Server struct {
	hd          server.Handler
	rsvc        *register.Service
	exit        chan chan error
	subscribers map[*tcpSubscriber][]broker.Subscriber
	opts        server.Options
	sync.RWMutex
	registered  bool
	init        bool
	stateLive   *atomic.Uint32
	stateReady  *atomic.Uint32
	stateHealth *atomic.Uint32
}

func (h *Server) Live() bool {
	return h.stateLive.Load() == 1
}

func (h *Server) Ready() bool {
	return h.stateReady.Load() == 1
}

func (h *Server) Health() bool {
	return h.stateHealth.Load() == 1
}

func (h *Server) newCodec(ct string) (codec.Codec, error) {
	if cf, ok := h.opts.Codecs[ct]; ok {
		return cf, nil
	}
	return nil, codec.ErrUnknownContentType
}

func (h *Server) Options() server.Options {
	h.RLock()
	defer h.RUnlock()
	return h.opts
}

func (h *Server) Init(opts ...server.Option) error {
	if h.opts.Wait == nil {
		h.opts.Wait = msync.NewWaitGroup()
	}
	if len(opts) == 0 && h.init {
		return nil
	}
	h.Lock()
	for _, o := range opts {
		o(&h.opts)
	}
	h.Unlock()

	if err := h.opts.Register.Init(); err != nil {
		return err
	}
	if err := h.opts.Broker.Init(); err != nil {
		return err
	}
	if err := h.opts.Tracer.Init(); err != nil {
		return err
	}
	if err := h.opts.Logger.Init(); err != nil {
		return err
	}
	if err := h.opts.Meter.Init(); err != nil {
		return err
	}

	return nil
}

func (h *Server) Handle(handler server.Handler) error {
	h.Lock()
	h.hd = handler
	h.Unlock()
	return nil
}

func (h *Server) NewHandler(handler interface{}, opts ...server.HandlerOption) server.Handler {
	options := server.NewHandlerOptions(opts...)

	th := &tcpHandler{
		hd:   handler,
		opts: options,
	}

	if size, ok := h.opts.Context.Value(maxMsgSizeKey{}).(int); ok && size > 0 {
		th.maxMsgSize = size
	}

	return th
}

func (h *Server) NewSubscriber(topic string, handler interface{}, opts ...server.SubscriberOption) server.Subscriber {
	return newSubscriber(topic, handler, opts...)
}

func (h *Server) Subscribe(sb server.Subscriber) error {
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

func (h *Server) Register() error {
	h.Lock()
	config := h.opts
	rsvc := h.rsvc

	h.Unlock()

	// if service already filled, reuse it and return early
	if rsvc != nil {
		if err := server.DefaultRegisterFunc(rsvc, config); err != nil {
			return err
		}
		return nil
	}

	service, err := server.NewRegisterService(h)
	if err != nil {
		return err
	}

	service.Nodes[0].Metadata["protocol"] = "tcp"
	service.Nodes[0].Metadata["transport"] = service.Nodes[0].Metadata["protocol"]

	h.Lock()

	subscriberList := make([]*tcpSubscriber, 0, len(h.subscribers))
	for e := range h.subscribers {
		// Only advertise non internal subscribers
		subscriberList = append(subscriberList, e)
	}
	sort.Slice(subscriberList, func(i, j int) bool {
		return subscriberList[i].topic > subscriberList[j].topic
	})

	h.Unlock()

	h.RLock()
	registered := h.registered
	h.RUnlock()

	if !registered {
		if config.Logger.V(logger.InfoLevel) {
			config.Logger.Info(config.Context, fmt.Sprintf("Register [%s] Registering node: %s", config.Register.String(), service.Nodes[0].ID))
		}
	}

	// register the service
	if err := server.DefaultRegisterFunc(service, config); err != nil {
		return err
	}

	// already registered? don't need to register subscribers
	if registered {
		return nil
	}

	h.Lock()
	defer h.Unlock()

	if h.registered {
		return nil
	}

	for sb := range h.subscribers {
		handler := h.createSubHandler(sb, config)
		var opts []broker.SubscribeOption
		if queue := sb.Options().Queue; len(queue) > 0 {
			opts = append(opts, broker.SubscribeGroup(queue))
		}

		subCtx := config.Context
		if cx := sb.Options().Context; cx != nil {
			subCtx = cx
		}
		opts = append(opts, broker.SubscribeContext(subCtx))
		opts = append(opts, broker.SubscribeAutoAck(sb.Options().AutoAck))

		if config.Logger.V(logger.InfoLevel) {
			config.Logger.Info(config.Context, "Subscribing to topic: "+sb.Topic())
		}

		sub, err := config.Broker.Subscribe(subCtx, sb.Topic(), handler, opts...)
		if err != nil {
			return err
		}
		h.subscribers[sb] = []broker.Subscriber{sub}
	}

	h.registered = true
	h.rsvc = service

	return nil
}

func (h *Server) Deregister() error {
	h.Lock()
	config := h.opts
	h.Unlock()

	service, err := server.NewRegisterService(h)
	if err != nil {
		return err
	}

	if config.Logger.V(logger.InfoLevel) {
		config.Logger.Info(config.Context, "Deregistering node: "+service.Nodes[0].ID)
	}

	if err := server.DefaultDeregisterFunc(service, config); err != nil {
		return err
	}

	h.Lock()
	if !h.registered {
		h.Unlock()
		return nil
	}
	h.registered = false

	wg := sync.WaitGroup{}
	subCtx := h.opts.Context

	for sb, subs := range h.subscribers {
		if cx := sb.Options().Context; cx != nil {
			subCtx = cx
		}

		for _, sub := range subs {
			wg.Add(1)
			go func(s broker.Subscriber) {
				defer wg.Done()
				if config.Logger.V(logger.InfoLevel) {
					config.Logger.Info(config.Context, "Unsubscribing from topic: "+s.Topic())
				}
				if err := s.Unsubscribe(subCtx); err != nil {
					if config.Logger.V(logger.ErrorLevel) {
						config.Logger.Error(config.Context, "Unsubscribing from errot topic: "+s.Topic(), err)
					}
				}
			}(sub)
		}
		h.subscribers[sb] = nil
	}
	wg.Wait()

	h.Unlock()
	return nil
}

func (h *Server) getListener() net.Listener {
	if h.opts.Context == nil {
		return nil
	}

	l, ok := h.opts.Context.Value(netListener{}).(net.Listener)
	if !ok || l == nil {
		return nil
	}

	return l
}

func (h *Server) Start() error {
	h.RLock()
	config := h.opts
	hd := h.hd.Handler()
	h.RUnlock()

	var err error
	var ts net.Listener

	if l := h.getListener(); l != nil {
		ts = l
	}

	// nolint: nestif
	if ts == nil {
		// check the tls config for secure connect
		if config.TLSConfig != nil {
			ts, err = tls.Listen("tcp", config.Address, config.TLSConfig)
			// otherwise just plain tcp listener
		} else {
			ts, err = net.Listen("tcp", config.Address)
		}
		if err != nil {
			return err
		}

		if config.Context != nil {
			if c, ok := config.Context.Value(maxConnKey{}).(int); ok && c > 0 {
				ts = netutil.LimitListener(ts, c)
			}
		}
	}

	if config.Logger.V(logger.ErrorLevel) {
		config.Logger.Info(config.Context, "Listening on "+ts.Addr().String())
	}

	h.Lock()
	h.opts.Address = ts.Addr().String()
	h.Unlock()

	if err = config.Broker.Connect(config.Context); err != nil {
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
	h.stateLive.Store(1)
	h.stateReady.Store(1)
	h.stateHealth.Store(1)

	go func() {
		t := new(time.Ticker)

		// only process if it exists
		if config.RegisterInterval > time.Duration(0) {
			// new ticker
			t = time.NewTicker(config.RegisterInterval)
		}

		// return error chan
		var ch chan error

	Loop:
		for {
			select {
			// register self on interval
			case <-t.C:
				h.RLock()
				registered := h.registered
				h.RUnlock()
				rerr := h.opts.RegisterCheck(h.opts.Context)
				// nolint: nestif
				if rerr != nil && registered {
					if config.Logger.V(logger.ErrorLevel) {
						config.Logger.Error(config.Context, fmt.Sprintf("Server %s-%s deregister, check error", config.Name, config.ID), rerr)
					}
					// deregister self in case of error
					if err := h.Deregister(); err != nil {
						if config.Logger.V(logger.ErrorLevel) {
							config.Logger.Error(config.Context, fmt.Sprintf("Server %s-%s deregister error", config.Name, config.ID), err)
						}
					}
				} else if rerr != nil && !registered {
					if config.Logger.V(logger.ErrorLevel) {
						config.Logger.Error(config.Context, fmt.Sprintf("Server %s-%s register check error", config.Name, config.ID), rerr)
					}
					continue
				}
				if err := h.Register(); err != nil {
					if config.Logger.V(logger.ErrorLevel) {
						config.Logger.Error(config.Context, fmt.Sprintf("Server %s-%s register error", config.Name, config.ID), err)
					}
				}
				// wait for exit
			case ch = <-h.exit:
				break Loop
			}
		}

		h.gracefulStop()

		ch <- ts.Close()

		h.stateLive.Store(0)
		h.stateReady.Store(0)
		h.stateHealth.Store(0)

		// deregister
		if cerr := h.Deregister(); cerr != nil {
			config.Logger.Error(config.Context, "Register deregister error", cerr)
		}

		if cerr := config.Broker.Disconnect(config.Context); cerr != nil {
			config.Logger.Error(config.Context, "Broker disconnect error", cerr)
		}
	}()

	return nil
}

func (h *Server) Stop() error {
	ch := make(chan error)
	h.exit <- ch
	return <-ch
}

func (h *Server) gracefulStop() {
	ctx, cancel := context.WithTimeout(context.Background(), h.opts.GracefulTimeout)
	defer cancel()

	h.opts.Wait.WaitContext(ctx)
}

func (h *Server) String() string {
	return "tcp"
}

func (h *Server) Name() string {
	return h.opts.Name
}

func (h *Server) serve(ln net.Listener, hd Handler) {
	var tempDelay time.Duration // how long to sleep on accept failure
	h.RLock()
	config := h.opts
	h.RUnlock()
	for {
		c, err := ln.Accept()
		// nolint: nestif
		if err != nil {
			select {
			case <-h.exit:
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				if config.Logger.V(logger.ErrorLevel) {
					config.Logger.Error(config.Context, fmt.Sprintf("tcp: Accept error: %v; retrying in %v", err, tempDelay))
				}
				time.Sleep(tempDelay)
				continue
			}
			if config.Logger.V(logger.ErrorLevel) {
				config.Logger.Error(config.Context, "tcp: Accept error", err)
			}
			return
		}

		if h.opts.Wait != nil {
			h.opts.Wait.Add(1)
		}
		go func() {
			hd.Serve(c)
			if h.opts.Wait != nil {
				h.opts.Wait.Done()
			}
		}()
	}
}

func NewServer(opts ...server.Option) server.Server {
	return &Server{
		stateLive:   &atomic.Uint32{},
		stateReady:  &atomic.Uint32{},
		stateHealth: &atomic.Uint32{},
		opts:        server.NewOptions(opts...),
		exit:        make(chan chan error),
		subscribers: make(map[*tcpSubscriber][]broker.Subscriber),
	}
}
