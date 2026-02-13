// Package grpchantest provides gRPC channel integration test infrastructure,
// adapted from github.com/fullstorydev/grpchan/grpchantesting (MIT License).
//
// Original copyright:
//
//	The MIT License (MIT)
//	Copyright (c) 2018 Fullstory, Inc
//
// This package provides a TestService implementation and RunChannelTestCases
// function that exercise all four gRPC call types (unary, client-streaming,
// server-streaming, bidirectional-streaming) with success, failure, timeout,
// and cancellation conditions.
package grpchantest
