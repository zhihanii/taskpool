package taskpool

import "time"

type Option func(o *Options)

func WithMaxBlockingTasks(maxBlockingTasks int) Option {
	return func(o *Options) {
		o.MaxBlockingTasks = maxBlockingTasks
	}
}

func WithNonBlocking() Option {
	return func(o *Options) {
		o.Nonblocking = true
	}
}

type Options struct {
	ExpiredDuration  time.Duration
	MaxBlockingTasks int
	Nonblocking      bool
}

func NewOptions() *Options {
	return &Options{
		ExpiredDuration: DefaultExpiredDuration,
		Nonblocking:     false,
	}
}

func (o *Options) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(o)
	}
}

//func NewDefaultOptions() *options {
//	return &options{
//		ExpiredDuration: DefaultExpiredDuration,
//	}
//}
