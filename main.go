package main

import (
	//"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc/credentials"

	message "github.com/emjakobsen1/security-handin-2/proto"
	"google.golang.org/grpc"
)

var nameToPort = map[string]int32{
	"Hospital": 5000,
	"Alice":    5001,
	"Bob":      5002,
	"Charlie":  5003,
}

var portToName = func() map[int32]string {
	m := make(map[int32]string)
	for name, port := range nameToPort {
		m[port] = name
	}
	return m
}()

var secretsMap = map[string]int32{
	"Hospital": 5000,
	"Alice":    5001,
	"Bob":      5002,
	"Charlie":  5003,
}

type peer struct {
	message.UnimplementedServiceServer
	id                   int32
	name                 string
	clients              map[int32]message.ServiceClient
	ctx                  context.Context
	receivedChunksOfData []int32
}

var R int = 10001
var hospitalId int32 = 5000

func main() {
	log.SetFlags(log.Lshortfile)
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + hospitalId //clients are 5001 5002 5003

	secret, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatalf("Invalid second argument: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &peer{
		id:                   ownPort,
		clients:              make(map[int32]message.ServiceClient),
		receivedChunksOfData: make([]int32, 0, 3),
		ctx:                  ctx,
		name:                 portToName[ownPort],
	}

	//set up server
	list, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", ownPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	f := setLog(ownPort)
	defer f.Close()

	var serverCertFile, serverKeyFile string
	switch p.name {
	case "Hospital":
		serverCertFile = "Hospital.crt"
		serverKeyFile = "Hospital.key"
	case "Alice":
		serverCertFile = "Alice.crt"
		serverKeyFile = "Alice.key"
	case "Bob":
		serverCertFile = "Bob.crt"
		serverKeyFile = "Bob.key"
	case "Charlie":
		serverCertFile = "Charlie.crt"
		serverKeyFile = "Charlie.key"
	}

	serverCert, err := credentials.NewServerTLSFromFile(serverCertFile, serverKeyFile)
	if err != nil {
		log.Fatalln("failed to create cert", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(serverCert))
	message.RegisterServiceServer(grpcServer, p)

	// start the server
	go func() {
		if err := grpcServer.Serve(list); err != nil {
			log.Fatalf("failed to serve %v", err)
		}
	}()

	//Dial the other peers
	for i := 0; i <= 3; i++ {
		port := hospitalId + int32(i)

		if port == ownPort {
			continue
		}

		var clientCertFile string
		clientName := portToName[port]
		switch clientName {
		case "Hospital":
			clientCertFile = "Hospital.crt"
		case "Alice":
			clientCertFile = "Alice.crt"
		case "Bob":
			clientCertFile = "Bob.crt"
		case "Charlie":
			clientCertFile = "Charlie.crt"
		}

		clientCert, err := credentials.NewClientTLSFromFile(clientCertFile, "")
		if err != nil {
			log.Fatalln("failed to create certificate", err)
		}

		fmt.Printf("Trying to dial: %v\n", port)
		conn, err := grpc.Dial(fmt.Sprintf("localhost:%v", port), grpc.WithTransportCredentials(clientCert), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect: %s", err)
		}
		defer conn.Close()
		c := message.NewServiceClient(conn)
		p.clients[port] = c

	}
	log.Printf("%v | Secret: %v \n", portToName[ownPort], secret)
	share1, share2, share3 := splitValue(secret, R)
	p.receivedChunksOfData = append(p.receivedChunksOfData, int32(share3))
	log.Printf("%v | Secret into shares: %v %v %v \n", portToName[ownPort], share1, share2, share3)

	// scanner := bufio.NewScanner(os.Stdin)
	// if ownPort != hospitalId {
	// 	fmt.Print("Enter a number between 0 and 1 000 000 to share it secretly with the other peers.\n Number: ")
	// 	for scanner.Scan() {
	// 		//num, _ := strconv.ParseInt(scanner.Text(), 10, 32)
	// 		p.ShareData(int(secret))
	// 	}
	// } else {
	// 	fmt.Print("Hospital | Waiting on peers \n")
	// 	for scanner.Scan() {
	// 		if scanner.Text() == "quit" {
	// 			return
	// 		}
	// 	}
	// }
	for {
		if ownPort != hospitalId {
			p.ShareData(int(share1))
			time.Sleep(10 * time.Second)

		}
		if len(p.receivedChunksOfData) == 3 {
			p.SendToHospital(int(p.sumChunks()))
			if p.name != "Hospital" {
				return
			}
		}
	}
}
func (p *peer) sumChunks() int32 {
	var sum int32 = 0
	for _, chunk := range p.receivedChunksOfData {
		sum += chunk
	}
	return sum
}

func splitValue(secret, R int) (int, int, int) {
	rand.Seed(time.Now().UnixNano())

	share1 := rand.Intn(R)
	share2 := rand.Intn(R)
	share3 := (secret - share1 - share2) % R

	if share3 < 0 {
		share3 += R
	}

	return share1, share2, share3
}
func (p *peer) ShareData(chunk int) {
	log.Printf("%s | Sending message to peers: %v", p.name, chunk)
	info := &message.Info{Id: p.id, SecretMessage: int32(chunk)}
	for id, client := range p.clients {
		peerName := portToName[id]
		if peerName == "Hospital" {
			continue
		}
		_, err := client.Request(p.ctx, info)
		if err != nil {
			log.Printf("%s | Failed to send message to peer %s: %v", p.name, peerName, err)
		}
	}
}

// Skal kun sende recievedChunksOFData når den er 3
func (p *peer) SendToHospital(chunk int) {
	log.Printf("%s | Sending message to the Hospital: %v", p.name, chunk)
	info := &message.Info{Id: p.id, SecretMessage: int32(chunk)}

	hospitalPort := int32(5000)
	hospitalClient, exists := p.clients[hospitalPort]
	if !exists {
		log.Printf("%s | Hospital client not found", p.name)
		return
	}

	_, err := hospitalClient.Request(p.ctx, info)
	if err != nil {
		log.Printf("%s | Failed to send message to the Hospital: %v", p.name, err)
	} else {
		log.Printf("%s | Successfully sent message to the Hospital", p.name)
	}
}

func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	peerName := portToName[req.Id]
	p.receivedChunksOfData = append(p.receivedChunksOfData, req.SecretMessage)
	log.Printf("%s | Received message from %s with %v", p.name, peerName, req.SecretMessage)
	log.Printf("%s | current chunks: %v", p.name, p.receivedChunksOfData)
	if p.name == "Hospital" {
		total := p.sumChunks()
		log.Printf("total sum %v", int(total)%R)
	}
	return &message.Empty{}, nil
}

func setLog(port int32) *os.File {
	logID := portToName[port]
	filename := fmt.Sprintf("logs/%v-log.txt", logID)
	if err := os.Truncate(filename, 0); err != nil {
		log.Printf("Failed to truncate: %v\n", err)
	}
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	log.SetFlags(0)
	log.SetOutput(mw)
	return f
}
