package taskbus

import (
	"context"
	"sync"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

type cronSvc struct {
	c    *client
	cron *cronv3.Cron
	mu   sync.Mutex
	ids  map[string]cronv3.EntryID
}

func newCronLocal(c *client) Cron {
	loc := time.Local
	if tz := c.cfg.Cron.Timezone; tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	cr := cronv3.New(cronv3.WithSeconds(), cronv3.WithLocation(loc))
	return &cronSvc{c: c, cron: cr, ids: make(map[string]cronv3.EntryID)}
}

func (s *cronSvc) Add(spec string, name string, fn func(context.Context) error, mws ...CronMiddleware) (string, error) {
	if fn == nil {
		return "", nil
	}
	final := fn
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}
	id, err := s.cron.AddFunc(spec, func() { _ = final(context.Background()) })
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := name
	if key == "" {
		key = spec
	}
	s.ids[key] = id
	return key, nil
}

func (s *cronSvc) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if eid, ok := s.ids[id]; ok {
		s.cron.Remove(eid)
		delete(s.ids, id)
	}
	return nil
}

func (s *cronSvc) Start(ctx context.Context) error { s.cron.Start(); return nil }

func (s *cronSvc) Stop(ctx context.Context) error { s.cron.Stop(); return nil }
