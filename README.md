# Security hand-in 2

This demo consists of 3 patients and the hospital communicating in a peer-to-peer environment over a localhost network. 

### Prerequisties 

It is possible you will need 1 or all of the following installed to run the demo:

- Go

- gRPC for Go (please see https://github.com/NaddiNadja/grpc101?tab=readme-ov-file#prerequisites for a guide)

- openSSL



##  How to run

 1. Run `main.go` in 4 separate terminals

    ```
    $ go run main.go 0 0 
    $ go run main.go 1 <secret integer>
    $ go run main.go 2 <secret integer>
    $ go run main.go 3 <secret integer>
    ```

It is hardcoded for 4 peers with Hospital port: 5000, Alice: 5001, Bob: 5002, Charlie: 5003. 


2. A peer will create a .txt with the events in the logs directory under the name `*-log.txt`, with `*` being the name. 
