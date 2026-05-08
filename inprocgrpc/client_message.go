package inprocgrpc

import "google.golang.org/grpc/metadata"

func (s *clientStreamAdapter) storeHeaders(md metadata.MD) error {
	s.metaMu.Lock()
	if s.headersRetrieved {
		s.metaMu.Unlock()
		return nil
	}
	s.headersRetrieved = true
	s.headers = cloneMetadata(md)
	s.copts.SetHeaders(s.headers)
	headers := cloneMetadata(s.headers)
	s.metaMu.Unlock()
	_ = s.stats.inHeader(headers, s.method)
	return nil
}

func (s *clientStreamAdapter) storeTrailers(md metadata.MD) error {
	s.metaMu.Lock()
	if s.trailersRetrieved {
		s.metaMu.Unlock()
		return nil
	}
	s.trailersRetrieved = true
	s.trailers = cloneMetadata(md)
	s.copts.SetTrailers(s.trailers)
	trailers := cloneMetadata(s.trailers)
	s.metaMu.Unlock()
	_ = s.stats.inTrailer(trailers)
	return nil
}

func (s *clientStreamAdapter) cloneMessage(message any) (any, error) {
	if s.cloneDisabled {
		return message, nil
	}
	result := cloneMessageSafe("clone request", s.cloner, message)
	return result.value, result.err
}

func (s *clientStreamAdapter) copyMessage(target, source any) error {
	if s.cloneDisabled {
		shallowCopy(target, source)
		return nil
	}
	return copyMessageSafe("copy response", s.cloner, target, source)
}
