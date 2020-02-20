package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"

	pb "github.com/grpc-ecosystem/go-grpc-prometheus/examples/grpc-server-with-prometheus/protobuf"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
)

/****
PROOF OF CONCEPT FOR PROMETHEUS METRICS
****/

type ServerMetrics struct {
	serverHandledCounter   *prom.CounterVec
	serverHandledHistogram *prom.HistogramVec
}

var SeverMetricLabels = []string{"grpc_service", "grpc_method", "param_name"}

func NewServerMetrics( /*counterOpts ...CounterOption*/ ) *ServerMetrics {
	//opts := counterOptions(counterOpts)
	return &ServerMetrics{
		serverHandledCounter: prom.NewCounterVec(
			prom.CounterOpts{
				Name: "grpc_server_handled_total",
				Help: "Total number of RPCs completed on the server, regardless of success or failure.",
			}, SeverMetricLabels,
		),
		serverHandledHistogram: prom.NewHistogramVec(
			prom.HistogramOpts{
				Name:    "grpc_server_handling_seconds",
				Help:    "Histogram of response latency (seconds) of gRPC that had been application-level handled by the server.",
				Buckets: prom.DefBuckets,
			}, SeverMetricLabels,
		),
	}
}

func (m *ServerMetrics) Describe(ch chan<- *prom.Desc) {
	m.serverHandledCounter.Describe(ch)
	m.serverHandledHistogram.Describe(ch)
}

func (m *ServerMetrics) Collect(ch chan<- prom.Metric) {
	m.serverHandledCounter.Collect(ch)
	m.serverHandledHistogram.Collect(ch)
}

// Casting
func CustomLabel(v interface{}) string {
	switch t := v.(type) {
	case *pb.HelloRequest:
		return t.Name
	default:
		return "unknown"
	}
}

// UnaryServerInterceptor is a gRPC server-side interceptor that provides Prometheus monitoring for Unary RPCs.
func (m *ServerMetrics) UnaryServerInterceptor() func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		service, method := splitMethodName(info.FullMethod)
		fmt.Printf("Type: %T\n", req)
		fmt.Printf("Value: %v\n", req)
		monitor := newServerReporter(m, service, method, CustomLabel(req))
		resp, err := handler(ctx, req)
		monitor.Handled()
		return resp, err
	}
}

type serverReporter struct {
	metrics     *ServerMetrics
	serviceName string
	methodName  string
	customLabel string
	startTime   time.Time
}

func splitMethodName(fullMethodName string) (string, string) {
	fullMethodName = strings.TrimPrefix(fullMethodName, "/") // remove leading slash
	if i := strings.Index(fullMethodName, "/"); i >= 0 {
		return fullMethodName[:i], fullMethodName[i+1:]
	}
	return "unknown", "unknown"
}

func newServerReporter(m *ServerMetrics, service, method, customLabel string) *serverReporter {
	r := &serverReporter{
		metrics:     m,
		serviceName: service,
		methodName:  method,
		customLabel: customLabel,
		startTime:   time.Now(),
	}
	return r
}

func (r *serverReporter) Handled() {
	r.metrics.serverHandledCounter.WithLabelValues(r.serviceName, r.methodName, r.customLabel).Inc()
	r.metrics.serverHandledHistogram.WithLabelValues(r.serviceName, r.methodName, r.customLabel).Observe(time.Since(r.startTime).Seconds())
}

/****
END OF POC FOR PROMETHEUS
****/

// DemoServiceServer defines a Server.
type DemoServiceServer struct{}

func newDemoServer() *DemoServiceServer {
	return &DemoServiceServer{}
}

// SayHello implements a interface defined by protobuf.
func (s *DemoServiceServer) SayHello(ctx context.Context, request *pb.HelloRequest) (*pb.HelloResponse, error) {
	return &pb.HelloResponse{Message: fmt.Sprintf("Hello %s", request.Name)}, nil
}

var (
	// Create a metrics registry.
	reg = prom.NewRegistry()

	// Create some standard server metrics.
	grpcMetrics = NewServerMetrics()

	serverInterceptors = []grpc.UnaryServerInterceptor{
		grpcMetrics.UnaryServerInterceptor(),
	}

	serverOptions = []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(serverInterceptors...),
	}

	// Create a customized counter metric.
	// customizedCounterMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
	//	Name: "demo_server_say_hello_method_handle_count",
	//	Help: "Total number of RPCs handled on the server.",
	// }, []string{"name"})
)

func init() {
	// Register standard server metrics and customized metrics to registry.
	reg.MustRegister(grpcMetrics)
	//customizedCounterMetric.WithLabelValues("Test")
}

// NOTE: Graceful shutdown is missing. Don't use this demo in your production setup.
func main() {
	// Listen an actual port.
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 9093))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// Create a HTTP server for prometheus.
	httpServer := &http.Server{Handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), Addr: fmt.Sprintf("0.0.0.0:%d", 9092)}

	// Create a gRPC Server with gRPC interceptor.
	grpcServer := grpc.NewServer(
		serverOptions...,
	)

	// Create a new api server.
	demoServer := newDemoServer()

	// Register your service.
	pb.RegisterDemoServiceServer(grpcServer, demoServer)

	// Initialize all metrics.
	//grpcMetrics.InitializeMetrics(grpcServer)

	// Start your http server for prometheus.
	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatal("Unable to start a http server.")
		}
	}()

	// Start your gRPC server.
	log.Fatal(grpcServer.Serve(lis))
}
