package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/logging"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var exporter = &sampleExporter{}

// Cloud Logging のロガーの初期化周りを行う
func gRPCLoggerStart(ctx context.Context, db string) {
	// db のパスから Project ID を取り出して Cloud Logging の Porject ID として利用
	id := strings.Split(db, "/")[1]
	cli, err := logging.NewClient(ctx, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed with %v", err)
		os.Exit(1)
	}
	logger := cli.Logger("spanner-request-log")

	exporter = &sampleExporter{
		projectID: id,
		client:    cli,
		logger:    logger,
	}
}

// Cloud Logging のロガーの終了処理を行う
func gRPCLoggerStop() {
	if exporter != nil {
		exporter.logger.Flush()
		exporter.client.Close()
	}
}

// Cloud Logging にわたす構造化ログ
type spannerClientLog struct {
	Method  string
	Message proto.Message
}

type sampleExporter struct {
	projectID string
	client    *logging.Client
	logger    *logging.Logger
}

// Cloud Logging にログを書き込む部分
func (exporter *sampleExporter) logMessage(method string, msg proto.Message) {
	exporter.logger.Log(logging.Entry{
		Payload: &spannerClientLog{
			Method:  method,
			Message: msg,
		},
		Severity: logging.Debug,
	})
	fmt.Fprintf(os.Stdout, "[%v]\n", method)
	return
}

// Cloud Spanner の Client にわたすための Interceptor を返す ClientOption
func getInterceptOpts(ctx context.Context) []option.ClientOption {
	if exporter.logger == nil {
		fmt.Fprintf(os.Stderr, "Execute gRPCLoggerStart() in the main function.\n")
		os.Exit(1)
	}
	opts := []option.ClientOption{
		option.WithGRPCDialOption(
			grpc.WithUnaryInterceptor(spannerUnaryClientInterceptor(exporter)),
		),
		option.WithGRPCDialOption(
			grpc.WithStreamInterceptor(spannerStreamClientInterceptor(exporter)),
		),
	}
	return opts
}

// Unary RPC（ExecuteSql など）のための Interceptor
func spannerUnaryClientInterceptor(exporter *sampleExporter) grpc.UnaryClientInterceptor {
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
			exporter.logMessage(method, msg)
		}
		return err
	}
}

// Streaming RPC（ExequteStreamingSql など） のための Interceptor
func spannerStreamClientInterceptor(exporter *sampleExporter) grpc.StreamClientInterceptor {
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
		return &loggingClientStream{exporter, method, nil, false, s}, err
	}
}

// Streming RPC の中で持ち回る構造体
type loggingClientStream struct {
	exporter *sampleExporter
	method   string
	msg      proto.Message
	logged   bool
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
	s.exporter.logMessage(s.method, s.msg)
	s.logged = true
	return err
}
