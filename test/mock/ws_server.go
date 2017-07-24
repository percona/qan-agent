/*
   Copyright (c) 2016, Percona LLC and/or its affiliates. All rights reserved.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package mock

import (
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/net/websocket"
)

func NewWebsocketServer() *WebsocketServer {
	return &WebsocketServer{
		Clients:                   NewClients(),
		internalClientConnectChan: make(chan *client),
		ClientConnectChan:         make(chan *client, 1),
	}
}

type WebsocketServer struct {
	Clients                   *clients
	internalClientConnectChan chan *client
	ClientConnectChan         chan *client
}

// addr: http://127.0.0.1:8000
// endpoint: /agent
func (s *WebsocketServer) Run(addr string, endpoint string) {
	go s.run()
	http.Handle(endpoint, websocket.Handler(s.wsHandler))
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

// addr: https://127.0.0.1:8443
// endpoint: /agent
func (s *WebsocketServer) RunWss(addr string, endpoint string) {
	go s.run()
	http.Handle(endpoint, websocket.Handler(s.wsHandler))
	curDir, _ := os.Getwd()
	curDir = strings.TrimSuffix(curDir, "client")
	if err := http.ListenAndServeTLS(addr, curDir+"test/keys/cert.pem", curDir+"test/keys/key.pem", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func (s *WebsocketServer) run() {
	for {
		select {
		case c := <-s.internalClientConnectChan:
			s.Clients.add(c)
			select {
			case s.ClientConnectChan <- c:
			default:
			}
		}
	}
}

func (s *WebsocketServer) wsHandler(ws *websocket.Conn) {
	c := &client{
		ws:             ws,
		origin:         ws.Config().Origin.String(),
		SendChan:       make(chan interface{}, 5),
		RecvChan:       make(chan interface{}, 5),
		disconnectChan: make(chan struct{}),
	}
	s.internalClientConnectChan <- c

	defer func() {
		close(c.disconnectChan)
	}()
	go c.send()
	c.recv()
}

type client struct {
	ws             *websocket.Conn
	origin         string
	SendChan       chan interface{} // data to client
	RecvChan       chan interface{} // data from client
	disconnectChan chan struct{}
}

func (c *client) recv() {
	defer c.ws.Close()
	for {
		var data interface{}
		err := websocket.JSON.Receive(c.ws, &data)
		if err != nil {
			break
		}
		c.RecvChan <- data
	}
}

func (c *client) send() {
	defer c.ws.Close()
	for data := range c.SendChan {
		// log.Printf("recv: %+v\n", data)
		err := websocket.JSON.Send(c.ws, data)
		if err != nil {
			break
		}
	}
}

func NewClients() *clients {
	return &clients{
		list: make(map[*client]*client),
	}
}

type clients struct {
	list map[*client]*client
	sync.RWMutex
}

func (cl *clients) add(c *client) {
	cl.Lock()
	defer cl.Unlock()

	cl.list[c] = c
}

func (cl *clients) del(c *client) {
	cl.Lock()
	defer cl.Unlock()

	if _, ok := cl.list[c]; ok {
		delete(cl.list, c)
		close(c.SendChan)
	}
}

func (cl *clients) get(c *client) *client {
	cl.RLock()
	defer cl.RUnlock()

	c, ok := cl.list[c]
	if ok {
		return c
	}
	return nil
}

func (cl *clients) getAny() *client {
	cl.RLock()
	defer cl.RUnlock()

	for _, c := range cl.list {
		return c
	}
	return nil
}

func (cl *clients) Disconnect(c *client) {
	c = cl.get(c)
	if c != nil {
		c.ws.Close()
		<-c.disconnectChan
		cl.del(c)
	}
}

// Disconnect all clients.
func (cl *clients) DisconnectAll() {
	for {
		c := cl.getAny()
		if c == nil {
			return
		}
		cl.Disconnect(c)
	}
}
