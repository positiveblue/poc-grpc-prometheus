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
	"github.com/grpc-ecosystem/go-grpc-prometheus/packages/grpcstatus"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
)

/****
PROOF OF CONCEPT FOR PROMETHEUS METRICS
****/type ServerMetrics struct {
	labels                 []string
	serverHandledCounter   *prom.CounterVec
	serverHandledHistogram *prom.HistogramVec
}

// NewServerMetrics returns a ServerMetric which exposes the grpc service metrics for prometheus.
// SeverMetricLabels should contain the name for the custom labels that we want to attach to all the
// metrics.
func NewServerMetrics(labelExtractor LabelExtractor) *ServerMetrics {
	labels := append([]string{"grpc_service", "grpc_method", "grpc_status"}, labelExtractor.LabelNames()...)
	return &ServerMetrics{
		labels: labels,
		serverHandledCounter: prom.NewCounterVec(
			prom.CounterOpts{
				Name: "grpc_server_handled_total",
				Help: "Total number of RPCs completed on the server, regardless of success or failure.",
			}, labels,
		),
		serverHandledHistogram: prom.NewHistogramVec(
			prom.HistogramOpts{
				Name:    "grpc_server_handling_seconds",
				Help:    "Histogram of response latency (seconds) of gRPC that had been application-level handled by the server.",
				Buckets: prom.DefBuckets,
			}, labels,
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

// LabelExtractor must extract the needed labels for each one of the metrics and return
// an array of labels in the SAME ORDER than the ServerMetricLabels used for creating a NewServerMetrics()
type LabelExtractor interface {
	LabelNames() []string
	Labels(context.Context) map[string]string
}

// DefaultLabelExtractor is a dummy LabelExtractor which returns the empty
// list when processing the context to get the CustomLabels
type DefaultLabelExtractor struct{}

// LabelNames returns the names of the extra labels per metric
func (d *DefaultLabelExtractor) LabelNames() []string {
	return []string{}
}

// Labels returns the empty list
func (d *DefaultLabelExtractor) Labels(ctx context.Context) map[string]string {
	res := map[string]string{}
	for _, l := range d.LabelNames() {
		res[l] = "default"
	}
	return res
}

// Method used for spliting the service/method names of a grpc service
func splitMethodName(fullMethodName string) (string, string) {
	fullMethodName = strings.TrimPrefix(fullMethodName, "/") // remove leading slash
	if i := strings.Index(fullMethodName, "/"); i >= 0 {
		return fullMethodName[:i], fullMethodName[i+1:]
	}
	return "unknown", "unknown"
}

func (m *ServerMetrics) metricLabels(labelExtractor LabelExtractor, ctx context.Context, info *grpc.UnaryServerInfo) map[string]string {
	service, method := splitMethodName(info.FullMethod)

	// Populate basic labels
	labels := map[string]string{
		"grpc_service": service,
		"grpc_method":  method,
	}

	// Populate custom labels
	for k, v := range labelExtractor.Labels(ctx) {
		labels[k] = v
	}

	// Populate non-initialized custom labels with default value
	for _, labelName := range m.labels {
		if _, ok := labels[labelName]; !ok {
			labels[labelName] = "default"
		}
	}
	return labels
}

// UnaryServerInterceptor is a gRPC server-side interceptor that provides Prometheus monitoring for Unary RPCs.
func (m *ServerMetrics) UnaryServerInterceptor(labelExtractor LabelExtractor) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		metricLabels := m.metricLabels(labelExtractor, ctx, info)
		monitor := newServerReporter(m, metricLabels)
		resp, err := handler(ctx, req)
		st, _ := grpcstatus.FromError(err)
		monitor.labels["grpc_status"] = st.Code().String()
		monitor.Handled()
		return resp, err
	}
}

type serverReporter struct {
	metrics   *ServerMetrics
	labels    map[string]string
	startTime time.Time
}

func newServerReporter(m *ServerMetrics, labels map[string]string) *serverReporter {
	r := &serverReporter{
		metrics:   m,
		labels:    labels,
		startTime: time.Now(),
	}
	return r
}

func (r *serverReporter) Handled() {
	var orderedLabels []string
	for _, labelName := range r.metrics.labels {
		orderedLabels = append(orderedLabels, r.labels[labelName])
	}

	r.metrics.serverHandledCounter.WithLabelValues(orderedLabels...).Inc()
	r.metrics.serverHandledHistogram.WithLabelValues(orderedLabels...).Observe(time.Since(r.startTime).Seconds())
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

type CustomLabelExtractor struct{}

func (d *CustomLabelExtractor) LabelNames() []string {
	return []string{"userName"}
}

// Labels returns the empty list
func (d *CustomLabelExtractor) Labels(ctx context.Context) map[string]string {
	return map[string]string{"userName": "jordi", "appVersion": "v0.5"}
}

var (
	// Create a metrics registry.
	reg = prom.NewRegistry()

	customLabelExtractor = CustomLabelExtractor{}

	// Create some standard server metrics.
	grpcMetrics = NewServerMetrics(&customLabelExtractor)

	serverInterceptors = []grpc.UnaryServerInterceptor{
		grpcMetrics.UnaryServerInterceptor(&customLabelExtractor),
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
