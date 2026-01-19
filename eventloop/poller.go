// Package eventloop provides I/O event registration.
//
// # I/O Registration
//
// The event loop supports registering file descriptors for I/O events using
// platform-native mechanisms:
//   - Linux: epoll
//   - Darwin/BSD: kqueue
//
// See poller_linux.go and poller_darwin.go for platform-specific implementations.
//
// # Usage
//
//	loop.RegisterFD(fd, EventRead, func(events IOEvents) {
//	    // Handle readable event
//	})
//
// # Safety
//
// Always call UnregisterFD before closing a file descriptor to prevent
// stale event delivery due to FD recycling.
package eventloop

// Note: RegisterFD, UnregisterFD, ModifyFD, and pollIO are implemented
// in platform-specific files:
//   - poller_linux.go (epoll)
//   - poller_darwin.go (kqueue)
