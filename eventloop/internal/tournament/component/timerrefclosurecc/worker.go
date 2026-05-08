package timerrefclosurecc

// workerObserver is a zero-value qualification seam for the source Promisify
// admission, settlement publication, and WaitGroup completion order.
type workerObserver struct {
	workerCommitted     func()
	workerStarted       func()
	workerReturned      func()
	settlementPublished func(error)
	settlementExecuted  func()
	beforeWorkerDone    func()
}

func (l *loop) promisify(task func()) error {
	return l.promisifyObserved(task, workerObserver{})
}

func (l *loop) promisifyObserved(task func(), observer workerObserver) error {
	l.promisifyMu.Lock()
	current := state(l.state.Load())
	if current == stateTerminating || current == stateTerminated || l.quiescing.Load() {
		l.promisifyMu.Unlock()
		return errTerminated
	}
	l.promisifyWg.Add(1)
	l.promisifyCount.Add(1)
	l.submissionEpoch.Add(1)
	if observer.workerCommitted != nil {
		observer.workerCommitted()
	}
	l.promisifyMu.Unlock()

	go func() {
		defer l.promisifyWg.Done()
		defer func() {
			l.promisifyCount.Add(-1)
			if l.autoExit {
				l.doWakeup()
			}
			if observer.beforeWorkerDone != nil {
				observer.beforeWorkerDone()
			}
		}()
		defer func() {
			_ = recover()
			err := l.submitToQueue(func() {
				if observer.settlementExecuted != nil {
					observer.settlementExecuted()
				}
			})
			if observer.settlementPublished != nil {
				observer.settlementPublished(err)
			}
		}()

		if observer.workerStarted != nil {
			observer.workerStarted()
		}
		task()
		if observer.workerReturned != nil {
			observer.workerReturned()
		}
	}()
	return nil
}
