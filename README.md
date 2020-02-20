First version of a Go gRPC Prometheus Interceptor

## What is this this?

`client/client.go` calls our gRPC server SayHello method with two different values for the method parameter `Name` 

`server/server.go` exposes and endpoint for the prometheus metrics and a grpc method SayHello which receives a `Name` param. The GRPC method has a unary interceptor for exposing two metrics: handled req counter and handled req histogram. The metrics are populated with three labels `req service`, `req method` and the value of the req param `Name`. For getting the value of req.Name we use a method `func CustomLable(v {}interface) string` which basically uses type assertion for casting the interface and getting the right value for the label. For each req type that we want to decorate with the custom label we will have to add a case in the switch statement. Oterwhise, the metrics are populated with the "unknown" label.

`prometheus.yaml`: prometheus configuration


## How to run this?
Open three terminals and run

```
prometheus --config.file=prometheus.yaml # prometheus bin should be in your path
```
```
go run server.go
```

```
go run client.go
```

Open your browser and go to `localhost:9090`.
