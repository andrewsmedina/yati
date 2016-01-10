// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

var LogPubSubQueuePrefix = "pubsub:"
var bulkMaxWaitTime = 500 * time.Millisecond

type LogListener struct {
	C <-chan Applog
	q queue.PubSubQ
}

func logQueueName(appName string) string {
	return LogPubSubQueuePrefix + appName
}

func NewLogListener(a *App, filterLog Applog) (*LogListener, error) {
	factory, err := queue.Factory()
	if err != nil {
		return nil, err
	}
	pubSubQ, err := factory.PubSub(logQueueName(a.Name))
	if err != nil {
		return nil, err
	}
	subChan, err := pubSubQ.Sub()
	if err != nil {
		return nil, err
	}
	c := make(chan Applog, 10)
	go func() {
		defer close(c)
		for msg := range subChan {
			applog := Applog{}
			err := json.Unmarshal(msg, &applog)
			if err != nil {
				log.Errorf("Unparsable log message, ignoring: %s", string(msg))
				continue
			}
			if (filterLog.Source == "" || filterLog.Source == applog.Source) &&
				(filterLog.Unit == "" || filterLog.Unit == applog.Unit) {
				defer func() {
					recover()
				}()
				c <- applog
			}
		}
	}()
	l := LogListener{C: c, q: pubSubQ}
	return &l, nil
}

func (l *LogListener) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recovered panic closing listener (possible double close): %#v", r)
		}
	}()
	err = l.q.UnSub()
	return
}

func notify(appName string, messages []interface{}) {
	factory, err := queue.Factory()
	if err != nil {
		log.Errorf("Error on logs notify: %s", err.Error())
		return
	}
	pubSubQ, err := factory.PubSub(logQueueName(appName))
	if err != nil {
		log.Errorf("Error on logs notify: %s", err.Error())
		return
	}
	for _, msg := range messages {
		bytes, err := json.Marshal(msg)
		if err != nil {
			log.Errorf("Error on logs notify: %s", err.Error())
			continue
		}
		err = pubSubQ.Pub(bytes)
		if err != nil {
			log.Errorf("Error on logs notify: %s", err.Error())
		}
	}
}

type logDispatcher struct {
	dispatchers map[string]*appLogDispatcher
}

func NewlogDispatcher() *logDispatcher {
	return &logDispatcher{
		dispatchers: make(map[string]*appLogDispatcher),
	}
}

func (d *logDispatcher) Send(msg *Applog) error {
	appName := msg.AppName
	appD := d.dispatchers[appName]
	if appD == nil {
		appD = newAppLogDispatcher(appName)
		d.dispatchers[appName] = appD
	}
	select {
	case appD.msgCh <- msg:
	case err := <-appD.errCh:
		close(appD.msgCh)
		delete(d.dispatchers, appName)
		return err
	}
	return nil
}

func (d *logDispatcher) Stop() error {
	var finalErr error
	for appName, appD := range d.dispatchers {
		delete(d.dispatchers, appName)
		close(appD.msgCh)
		err := <-appD.errCh
		if err != nil {
			if finalErr == nil {
				finalErr = err
			} else {
				finalErr = fmt.Errorf("%s, %s", finalErr, err)
			}
		}
	}
	return finalErr
}

type appLogDispatcher struct {
	appName string
	msgCh   chan *Applog
	errCh   chan error
	done    chan bool
	toFlush chan *Applog
}

func newAppLogDispatcher(appName string) *appLogDispatcher {
	d := &appLogDispatcher{
		appName: appName,
		msgCh:   make(chan *Applog, 10000),
		errCh:   make(chan error),
		done:    make(chan bool),
		toFlush: make(chan *Applog),
	}
	go d.runDBWriter()
	go d.runFlusher()
	return d
}

func (d *appLogDispatcher) runFlusher() {
	defer close(d.errCh)
	t := time.NewTimer(bulkMaxWaitTime)
	bulkBuffer := make([]interface{}, 0, 100)
	conn, err := db.LogConn()
	if err != nil {
		d.errCh <- err
		return
	}
	defer conn.Close()
	coll := conn.Logs(d.appName)
	for {
		var flush bool
		select {
		case <-d.done:
			return
		case msg := <-d.toFlush:
			bulkBuffer = append(bulkBuffer, msg)
			flush = len(bulkBuffer) == cap(bulkBuffer)
			if !flush {
				t.Reset(bulkMaxWaitTime)
			}
		case <-t.C:
			flush = len(bulkBuffer) > 0
		}
		if flush {
			err := coll.Insert(bulkBuffer...)
			if err != nil {
				d.errCh <- err
				return
			}
			bulkBuffer = bulkBuffer[:0]
		}
	}
}

func (d *appLogDispatcher) runDBWriter() {
	defer close(d.done)
	notifyMessages := make([]interface{}, 1)
	for msg := range d.msgCh {
		notifyMessages[0] = msg
		notify(msg.AppName, notifyMessages)
		d.toFlush <- msg
	}
}
