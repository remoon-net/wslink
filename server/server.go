package server

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/hashicorp/yamux"
	"github.com/maypok86/otter"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"nhooyr.io/websocket"
)

type Server struct {
	hub otter.Cache[string, *httputil.ReverseProxy]
}

var _ http.Handler = (*Server)(nil)

func New() *Server {
	hub := try.To1(
		otter.
			MustBuilder[string, *httputil.ReverseProxy](10_000).
			Build(),
	)
	srv := &Server{
		hub: hub,
	}
	return srv
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/")
	chunk := strings.SplitN(path, "/", 2)
	peer := chunk[0]
	if b, _ := hex.DecodeString(peer); len(b) != 32 {
		http.Error(w, "peer id 不规范", http.StatusBadRequest)
		return
	}
	if len(chunk) == 1 || chunk[1] == "" {
		srv.RegisterHandler(w, r, peer)
		return
	}
	srv.linkHandler(w, r, peer)
}

func (srv *Server) RegisterHandler(w http.ResponseWriter, r *http.Request, peer string) (err error) {
	defer err0.Then(&err, nil, func() {
		http.Error(w, err.Error(), 500)
	})
	hub := srv.hub
	if hub.Has(peer) {
		return fmt.Errorf("该地址已被使用")
	}
	socket := try.To1(websocket.Accept(w, r, nil))
	ctx := r.Context()
	conn := websocket.NetConn(ctx, socket, websocket.MessageBinary)
	sess := try.To1(yamux.Client(conn, nil))
	endpoint := fmt.Sprintf("http://yamux.proxy/")
	target := try.To1(url.Parse(endpoint))
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return sess.Open()
		},
	}
	ok := hub.SetIfAbsent(peer, proxy)
	if !ok {
		return fmt.Errorf("容量不够了")
	}
	defer hub.Delete(peer)
	<-sess.CloseChan()
	return nil
}

func (srv *Server) linkHandler(w http.ResponseWriter, r *http.Request, peer string) (err error) {
	defer err0.Then(&err, nil, func() {
		http.Error(w, err.Error(), 500)
	})
	hub := srv.hub
	proxy, ok := hub.Get(peer)
	if !ok || proxy == nil {
		return fmt.Errorf("不存在")
	}
	prefix := "/" + peer
	r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
	r.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, prefix)
	proxy.ServeHTTP(w, r)
	return nil
}
