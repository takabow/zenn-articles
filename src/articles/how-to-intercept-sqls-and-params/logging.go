package main

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/logging"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// Cloud Logging にわたす構造化ログ
type spannerClientLog struct {
	Method  string
	Message proto.Message
}

// Cloud Logging にログを書き込む部分
func logMessage(logger *logging.Logger, method string, msg proto.Message) {
	logger.Log(logging.Entry{
		Payload: &spannerClientLog{
			Method:  method,
			Message: msg,
		},
		Severity: logging.Debug,
	})
	fmt.Fprintf(os.Stdout, "[%v]\n", method)
	return
}

// Unary RPC（ExecuteSql など）のための Interceptor
func spannerUnaryClientInterceptor(logger *logging.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// ここで実際のリクエストを送信する
		err := invoker(ctx, method, req, reply, cc, opts...)
		// リクエストで送った msg を記録する
		if msg, ok := req.(proto.Message); ok {
			logMessage(logger, method, msg)
		}
		return err
	}
}

// Streaming RPC（ExequteStreamingSql など） のための Interceptor
func spannerStreamClientInterceptor(logger *logging.Logger) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		s, err := streamer(ctx, desc, cc, method, opts...)
		// 実際の割り込み処理は SendMsg と RecvMsg でそれぞれ行われる
		return &loggingClientStream{logger, method, nil, false, s}, err
	}
}

// Streming RPC の中で持ち回る構造体
type loggingClientStream struct {
	logger *logging.Logger
	method string
	msg    proto.Message
	logged bool
	grpc.ClientStream
}

// Streaming RPC のリクエスト送信時の割り込み処理
func (s *loggingClientStream) SendMsg(m interface{}) error {
	if msg, ok := m.(proto.Message); ok {
		s.msg = msg
	}
	return s.ClientStream.SendMsg(m)
}

// Streaming RPC のレスポンス受信時の割り込み処理
func (s *loggingClientStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	// RecvMsg は複数回呼ばれるので、最初の1つめでのみ記録
	if s.logged {
		return err
	}

	// レスポンス受信が始まったら当初のリクエストを記録する
	logMessage(s.logger, s.method, s.msg)
	s.logged = true
	return err
}
