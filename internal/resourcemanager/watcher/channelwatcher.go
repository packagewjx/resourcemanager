package watcher

type baseChannelWatcher struct {
	channels []chan *ProcessGroupStatus
}

func (b *baseChannelWatcher) notifyAll(s *ProcessGroupStatus) {
	for _, ch := range b.channels {
		ch <- s
	}
}

func (b *baseChannelWatcher) stopAll() {
	for _, channel := range b.channels {
		close(channel)
	}
}

func (b *baseChannelWatcher) Watch() <-chan *ProcessGroupStatus {
	ch := make(chan *ProcessGroupStatus, 1)
	b.channels = append(b.channels, ch)
	return ch
}

func (b *baseChannelWatcher) StopWatch(ch <-chan *ProcessGroupStatus) {
	for i := 0; i < len(b.channels); i++ {
		if b.channels[i] == ch {
			// FIXME 有竞争问题
			close(b.channels[i])
			b.channels = append(b.channels[:i], b.channels[i+1:]...)
			return
		}
	}
}
