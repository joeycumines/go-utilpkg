//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package eventloop

import "golang.org/x/sys/unix"

func pollerNativeFD(p *fastPoller) int { return int(p.kq) }

func pollerCreateNative() (int, error) { return unix.Kqueue() }
