package eventloop

// Note: RegisterFD, UnregisterFD, ModifyFD, and pollIO are implemented
// in platform-specific files:
//   - poller_linux.go (epoll)
//   - poller_darwin.go (kqueue)
