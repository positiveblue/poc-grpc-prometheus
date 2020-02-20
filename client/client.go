package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"

	pb "github.com/grpc-ecosystem/go-grpc-prometheus/examples/grpc-server-with-prometheus/protobuf"
)

func main() {

	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%v", 9093),
		grpc.WithInsecure(),
	)
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	// Create a gRPC server client.
	client := pb.NewDemoServiceClient(conn)
	fmt.Println("Start to call the method called SayHello every 3 seconds")
	go func() {
		for {
			// Call “SayHello” method and wait for response from gRPC Server.
			_, err := client.SayHello(context.Background(), &pb.HelloRequest{Name: "Test"})
			if err != nil {
				log.Printf("Calling the SayHello method unsuccessfully. ErrorInfo: %+v", err)
				log.Printf("You should to stop the process")
				return
			}
			time.Sleep(3 * time.Second)

			_, err = client.SayHello(context.Background(), &pb.HelloRequest{Name: "Test111"})
			if err != nil {
				log.Printf("Calling the SayHello method unsuccessfully. ErrorInfo: %+v", err)
				log.Printf("You should to stop the process")
				return
			}
			time.Sleep(3 * time.Second)
		}
	}()
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("You can press n or N to stop the process of client")
	for scanner.Scan() {
		if strings.ToLower(scanner.Text()) == "n" {
			os.Exit(0)
		}
	}
}
