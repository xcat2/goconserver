package common

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
)

var (
	signalSet *SignalSet
)

func DoSignal(done <-chan struct{}) {
	s := GetSignalSet()
	for {
		c := make(chan os.Signal)
		var sigs []os.Signal
		s.rwLock.RLock()
		for sig := range s.GetSigMap() {
			sigs = append(sigs, sig)
		}
		s.rwLock.RUnlock()
		signal.Notify(c, sigs...)
		select {
		case sig := <-c:
			err := s.Handle(sig, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "unknown signal received: %v\n", sig)
				os.Exit(1)
			}
		case <-done:
			return
		}
	}
}

type SignalHandler func(s os.Signal, arg interface{})

type SignalSet struct {
	rwLock *sync.RWMutex
	m      map[os.Signal]SignalHandler
}

func GetSignalSet() *SignalSet {
	if signalSet == nil {
		signalSet = new(SignalSet)
		signalSet.m = make(map[os.Signal]SignalHandler)
		signalSet.rwLock = new(sync.RWMutex)
	}
	return signalSet
}

func (set *SignalSet) Register(s os.Signal, handler SignalHandler) {
	set.rwLock.Lock()
	set.m[s] = handler
	set.rwLock.Unlock()
}

func (set *SignalSet) Handle(sig os.Signal, arg interface{}) (err error) {
	set.rwLock.RLock()
	defer set.rwLock.RUnlock()
	if _, found := set.m[sig]; found {
		set.m[sig](sig, arg)
		return nil
	} else {
		return fmt.Errorf("No handler available for signal %v", sig)
	}
}

func (set *SignalSet) GetSigMap() map[os.Signal]SignalHandler {
	return set.m
}
