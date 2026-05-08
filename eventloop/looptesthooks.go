package eventloop

type loopTestHooks struct {
	PrePollSleep                         func()                                // Called before CAS to StateSleeping
	PrePollAwake                         func()                                // Called before CAS back to StateRunning
	OnFastPathEntry                      func()                                // Called when entering fast path (runFastPath or direct exec)
	BeforeFastPathAutoExitReturn         func()                                // Called after fast-path auto-exit observes no liveness, before returning to run
	BeforeSetFastPathModeLock            func()                                // Called immediately before SetFastPathMode acquires liveness ownership
	BeforeSetQuiescenceHandlerLock       func()                                // Called immediately before SetQuiescenceHandler acquires liveness ownership
	BeforeTerminateState                 func()                                // Called after choosing termination, before StateTerminated is stored
	AfterShutdownStateTerminating        func()                                // Called after public Shutdown commits StateTerminating
	BeforeShutdownLifecycleLock          func()                                // Called after Shutdown observes a nonterminal state, before transition ownership
	AfterCloseStateTerminating           func()                                // Called after Close commits StateTerminating, before StateTerminated
	TerminalStateCAS                     func()                                // Called after StateTerminating CAS while terminalDrainMu remains locked; must not re-enter the loop
	BeforeTerminalModeLock               func()                                // Called before a mode reader synchronizes on terminalDrainMu; must not re-enter the loop
	BeforeRunLifecycleLock               func()                                // Called immediately before Run acquires liveness ownership
	BeforeBindJSLifecycleLock            func()                                // Called immediately before BindJS acquires liveness ownership
	AfterRunStateRunningBeforeStart      func()                                // Called after Run commits StateRunning, before runStarted publication
	BeforeTerminalJoin                   func()                                // Called after a non-owning external lifecycle caller commits to joining terminal completion
	AfterShutdownJoinContext             func()                                // Called after a joined Shutdown selects ctx.Done, before its terminalDone precedence recheck
	AfterTerminalDoneClose               func()                                // Called immediately after terminalDone closes
	BeforeClosePromiseRejection          func()                                // Called after Close publishes StateTerminated, before rejecting registered promises
	AfterTerminateStateBeforeDrain       func()                                // Called after StateTerminated is stored, before terminal queues drain
	BeforePromiseHandlerRegister         func()                                // Called after a pending rejection handler is stored, before handled registration
	AfterPromiseHandlerPendingCheck      func()                                // Called after addHandler observes a non-stable state, before its locked recheck
	AfterPromiseToChannelPendingCheck    func()                                // Called after ToChannel observes pending, before JS side-table registration
	BeforePromiseRejectLock              func()                                // Called immediately before reject attempts to acquire the promise lock
	AfterPromiseRejectionRecorded        func()                                // Called after a rejected promise is recorded, before Rejected is published
	AfterPromiseRejectedStateStore       func()                                // Called after Rejected is published and unlocked, before scheduling its check
	AfterPromiseReactionRegister         func()                                // Called after terminal reaction ownership is registered, before queue admission
	AfterPromiseHandlerScheduled         func()                                // Called after a promise reaction is queued
	BeforePromiseReactionClaim           func(*ChainedPromise)                 // Called after a Promise reaction is dequeued, before terminal-ownership claim
	BeforeAutoExitCommit                 func()                                // Called during auto-exit quiescing before the final Alive recheck
	AfterAutoExitFinalAliveCheck         func()                                // Called after auto-exit's last Alive recheck, before terminal admission is closed
	BeforeAutoExitTerminalDrainCommit    func()                                // Called while final auto-exit admission locks are held, immediately before terminal drain commits
	BeforeRunTimers                      func()                                // Called immediately before the normal timer phase enters runTimers
	BeforeScheduleTimerCommit            func()                                // Called after ScheduleTimer prepares a timer, before external liveness commit
	BeforeScheduleTimerReturn            func(TimerID)                         // Called after timer admission, immediately before callback publication and return
	BeforeTimerExecutionClaim            func(TimerID)                         // Called from a due timer immediately before callback-entry arbitration; must not re-enter the loop
	AfterTimerExecutionIngressDrain      func(TimerID)                         // Called with externalMu held after earlier commands are applied, before cancellation decides callback entry; must not re-enter the loop
	BeforeTimerPublicationWait           func(TimerID)                         // Called from a due timer immediately before waiting for ScheduleTimer publication
	BeforeAbortTimeoutClaim              func()                                // Called from a due AbortTimeout callback before manual/timer settlement arbitration
	AfterAbortTimeoutClaim               func()                                // Called after a due AbortTimeout callback wins settlement, before signal publication; must not panic, Goexit, or re-enter Abort
	AfterAbortTimeoutManualClaim         func()                                // Called after a manual Abort wins timeout settlement, before signal publication; must not panic, Goexit, or re-enter Abort
	BeforeAbortTimeoutPublicationWait    func()                                // Called after an Abort loses timeout settlement, immediately before waiting for publication
	BeforeTimerRefCommit                 func()                                // Called after RefTimer gates liveness, before external ref commit
	AfterSynchronousTimerCommandPublish  func(loopCommandKind)                 // Called after a result-bearing timer command is published and admission locks are released, before its caller waits
	BeforeRegisterFDRollbackCheck        func()                                // Called after RegisterFD poller registration, before pre-count rollback check
	BeforeRegisterFDCommit               func()                                // Called after RegisterFD poller registration/recheck, before count commit
	BeforeRegisterFDReturn               func(int)                             // Called after registration admission, immediately before callback publication and return
	BeforeFDPublicationCheck             func(int)                             // Called from a converted ready event immediately before checking RegisterFD publication
	RegisterFDRollback                   func(fd int) error                    // Replaces poller unregister only for deterministic RegisterFD rollback tests
	BeforeWakeFDRegister                 func(readFD, writeFD int)             // Called after wake descriptors and native poller creation, before wake registration
	BeforeFDModify                       func()                                // Called with Loop FD/liveness ownership immediately before poller ModifyFD
	BeforeFDUnregisterLock               func()                                // Called immediately before UnregisterFD acquires FD/liveness ownership
	BeforeFDUnregister                   func()                                // Called with Loop FD/liveness ownership immediately before poller UnregisterFD
	BeforePromisifyCommit                func()                                // Called after Promisify gates liveness, before goroutine tracking commit
	BeforePromisifyWorkerStart           func()                                // Called after worker launch, before it claims permission to enter user code
	AfterPromisifyWorkerEntryClaim       func()                                // Called after entry claim, before the first user-function instruction
	BeforePromisifyWorkerWake            func()                                // Called after worker completion, before its auto-exit wake is lifecycle-gated
	BeforeAliveEpochValidation           func()                                // Called after Alive observes no liveness, immediately before its final epoch validation
	BeforeCallbackAdmission              func()                                // Called after normal callback dequeue, before immediate-Close admission
	AfterReadyEventDispatchClaim         func(int)                             // Called with fd after dispatch claims a pending start, before callback admission
	BeforeCheckPredicateAdmission        func()                                // Called before a dynamic check/immediate liveness predicate enters callback admission
	BeforeUnhandledRejectionCallback     func()                                // Called before an unhandled-rejection user callback enters its selected admission path
	BeforeUnhandledRejectionRecordCheck  func(*ChainedPromise)                 // Called before a snapshotted rejection record is evaluated
	AfterUnhandledRejectionCheckClear    func()                                // Called after an active rejection checker clears its scheduled flag
	BeforeUnhandledRejectionRerunRequest func()                                // Called after detecting an active checker, before synchronizing rerun publication
	AfterUnhandledRejectionFallbackRerun func()                                // Called after terminal fallback collides with an active normal checker
	BeforeRejectionCheckScheduleClaim    func()                                // Called after the scheduled fast check, before serializing the generation claim
	BeforeCloseLifecycleLock             func()                                // Called before Close acquires livenessMu; callback admission remains open
	BeforeCommandIngressPublish          func(loopCommandKind)                 // Called with externalMu held after final admission succeeds, before command publication; must not re-enter ingress
	AfterCommandIngressPublish           func(loopCommandKind)                 // Called with externalMu held after pending state, command, and epoch publication; must not re-enter ingress or owner-materializing APIs
	AfterCommandIngressPopBeforeApply    func(loopCommandKind)                 // Called with externalMu held after a command is popped, before destination publication; must not re-enter ingress or owner-materializing APIs
	BeforeExternalPressureCheck          func()                                // Called after external phase snapshot drains, before remaining-pressure accounting
	BeforeTerminalDrainFinish            func()                                // Called with terminalDrainMu held just before a drain generation is closed
	BeforeTerminalEphemeralDrainSync     func()                                // Called before terminal ephemeral admission synchronizes on terminalDrainMu
	BeforeJSTimeoutRegistryPublish       func(uint64)                          // Called after JS.SetTimeout schedules its loop timer, before JS ID map publication
	BeforeJSTimeoutPublicationWait       func()                                // Called from a due JS timeout immediately before waiting for adapter-handle publication
	BeforeJSTimeoutCallbackClaim         func()                                // Called when a due JS timeout enters its wrapper, before claiming the adapter handle
	BeforeJSIntervalTimerIDPublish       func(uint64, *intervalState, TimerID) // Called after JS.SetInterval schedules its first loop timer, before current timer ID publication
	BeforeJSIntervalPublicationWait      func()                                // Called from a due JS interval immediately before waiting for adapter-handle publication
	BeforeJSIntervalCallbackEntry        func(uint64)                          // Called after a JS interval claims its callback, before user callback entry
	BeforeJSImmediateReturn              func(uint64)                          // Called after JS.SetImmediate admission, before callback publication is released
	BeforeJSImmediatePublicationWait     func()                                // Called from a JS immediate immediately before waiting for successful-call publication
	BeforeJSImmediateCallbackEntry       func(uint64)                          // Called after immediate execution claims its handle, before user callback entry
	AfterJSTimerPromiseRegister          func()                                // Called after terminal settlement registration, before native timer publication
	BeforeJSTimerPromiseCallbackFinish   func()                                // Called from a native timer callback before it claims promise settlement
	BeforeJSTerminalCleanupCollect       func()                                // Called with livenessMu held before terminal cleanup resolves weak JS registrations; must not re-enter liveness APIs
	AfterJSTerminalSettlementCollect     func()                                // Called after terminal cleanup strongly collects JS settlements, before native timer callbacks are cleared
	BeforeJSAdapterCleanup               func()                                // Called from runtime cleanup before JS registration retirement
	AfterJSAdapterCleanupLock            func()                                // Called with livenessMu held before JS registration retirement; must not re-enter liveness APIs
	PollError                            func() error                          // Injects poll error for testing handlePollError
	OnSubmitWakeup                       func()                                // Called when submitWakeup() is invoked (for testing pipe write optimization)
	BeforePhysicalWake                   func()                                // Called with wake-resource ownership before platform wake I/O
	BeforeWakeResourceClose              func()                                // Called before terminal cleanup joins admitted physical wakes
	BeforeCloseFDLock                    func()                                // Called immediately before terminal resource cleanup acquires fdMu
	BeforePendingWakeLock                func()                                // Called after physical pending fast rejection, before wakeMu acquisition
	BeforePollIO                         func()                                // Called at the final native-poll boundary before the terminal-state check
	PollIO                               func(int) (int, error)                // Replaces native PollIO while preserving the selected timeout
	BeforeFastPollWait                   func(int)                             // Called after initial channel drain, immediately before a blocking fast wait
	AfterWakeDrain                       func()                                // Called after a published wake descriptor is drained and its epoch is idle
	ReadWakeFD                           func(int, []byte) (int, error)        // Replaces the Unix wake read for deterministic error testing
	WriteWakeFD                          func(int, []byte) (int, error)        // Replaces the Unix wake write for deterministic error/race testing
}
