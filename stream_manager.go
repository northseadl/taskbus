package taskbus

import (
    "context"
    "fmt"
)

// StreamManager 管理多个独立的 Stream MQ 实例（每个实例对应一个独立的 host/DB）
type StreamManager interface {
    GetStream(name string) (MQ, error)
    Close(ctx context.Context) error
}

type streamManager struct {
    streams map[string]MQ
}

func newStreamManager(cfg Config, logger Logger) (StreamManager, error) {
    mgr := &streamManager{streams: map[string]MQ{}}
    if len(cfg.Stream.Streams) == 0 {
        return mgr, nil
    }
    for name, sc := range cfg.Stream.Streams {
        var (
            mq  MQ
            err error
        )
        switch sc.Provider {
        case MQProviderRabbitMQ:
            mode := sc.RabbitMQ.DelayMode
            if mode == "" {
                mode = DelayModeStandard
            }
            mq, err = newRabbitMQAdapterWithMode(sc.RabbitMQ, mode, RetryConfig{}, logger)
        case MQProviderRedis:
            mq, err = newRedisAdapter(sc.Redis, RetryConfig{}, logger)
        default:
            err = fmt.Errorf("unsupported stream provider: %s", sc.Provider)
        }
        if err != nil {
            return nil, fmt.Errorf("init stream %s: %w", name, err)
        }
        mgr.streams[name] = mq
    }
    return mgr, nil
}

func (m *streamManager) GetStream(name string) (MQ, error) {
    if q, ok := m.streams[name]; ok {
        return q, nil
    }
    return nil, fmt.Errorf("stream not found: %s", name)
}

func (m *streamManager) Close(ctx context.Context) error {
    for _, q := range m.streams {
        _ = q.Close(ctx)
    }
    return nil
}


