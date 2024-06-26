package client

import (
	"context"
	"net/http"

	"github.com/hashicorp/yamux"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"nhooyr.io/websocket"
)

type Client struct {
	handler http.Handler
}

var _ http.Handler = (*Client)(nil)

func New(handler http.Handler) *Client {
	return &Client{
		handler: handler,
	}
}

func (c *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.handler.ServeHTTP(w, r)
}

func (c *Client) Connect(ctx context.Context, link string, id string) (sess *yamux.Session, err error) {
	defer err0.Then(&err, nil, nil)
	socket, _ := try.To2(websocket.Dial(ctx, link, &websocket.DialOptions{
		Subprotocols: []string{"link", id},
	}))
	conn := websocket.NetConn(ctx, socket, websocket.MessageBinary)
	sess = try.To1(yamux.Server(conn, nil))
	go http.Serve(sess, c)
	return sess, nil
}
