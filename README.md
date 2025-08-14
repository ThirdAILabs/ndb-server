# NDB Server

A simple containerized service for ThirdAI NeuralDB.

## Usage

### Running Natively
To run natively you can build the executable with `go build cmd/main.go`. The resulting executable supports the following cli args:
```
Usage of ./main:
  -leader
    	Run as leader
  -s3-bucket string
    	S3 bucket name for checkpoints, optional, if not using no checkpoints will be pushed
  -s3-region string
    	S3 region for checkpoints, required if s3-bucket is specified
  -ckpt-interval string
    	Interval for checkpoints (e.g., 5m, 1h5m) (default "1h")
  -ckpt-dir string
    	Local directory to store checkpoints (default "./checkpoints")
  -max-ckpts int
    	Maximum number of checkpoints to keep in S3 (default 10)
  -tls
    	Enable TLS for the server, ensure tls-crt and tls-key are specified correctly
  -tls-crt string
    	Path to TLS certificate file (default "/certs/server.crt")
  -tls-key string
    	Path to TLS key file (default "/certs/server.key")
  -port int
    	Port to run the server on (default 80 for http or 443 for https if TLS is enabled)
```
### Running with Docker
1. Build the docker image:
```bash
docker build -t ndb-server -f DockerfileAmd64 .
```
2. Run the image (has the came cli args as mentioned above): 
```bash
docker run --rm -p 80:80 ndb-server --leader --s3-bucket <s3 bucket>
```
__Note:__ There are two dockerfiles available to use. The difference is that in DockerfileAmd64 a distroless debian image is used for the release stage, which reduces the image size from ~800Mb to ~50Mb. However it requires copying various libraries like libc, libgcc, etc from the build stage which uses the official go image. The paths of these libraries is specific to the architecture, and thus thus dockerfile only works for amd64 machines. The same image could be adapted to work on arm64, it would just require updating these paths.

### Using HTTPS/TLS
To use have the server support https intead of tls you must specify the `tls` flag (see above) as well as make sure that certificate and key files are provided (see above as well). For example if you are running on an ec2 instance then the following steps can be used to enable https with a self signed certificate. 
1. Fill in the DNS (will look something like ec2-xx-xx-xx-xx.compute-1.amazonaws.com) or IP address info in the `cert.sh` script.
2. Run `bash cert.sh` to generate the self signed certificates (requires openssl to be installed). 
3. Start the server with the tls flag and the certificates mounted: 
```bash
docker run --rm -p 443:443 -v ./certs:/certs ndb-server --leader --tls
```
4. Test to make sure everything is working: `curl -k -X GET https://<addr>/api/v1/health`. Note that the `-k` option is needed to or it will complain that the certificate was not issued by a known CA.

## Design 

### Endpoints

The container will expose the following endpoints:

- `/api/v1/health` - health check
- `/api/v1/search` - runs query
- `/api/v1/delete` - deletes documents
- `/api/v1/insert` - inserts documents
- `/api/v1/upvote` - apples finetuning for future queries
- `/api/v1/sources` - returns list of documents in NDB
- `/api/v1/version` - returns the current checkpoint version and the status of the last checkpoint
- `/api/v1/checkpoint` - pushes latest checkpoint (see details on checkpoints below)

See `docs/api_docs.md` for more detailed documentation on the endpoints.

### Model Checkpoints

- The docker container will require an s3 bucket name as an argument
- It will periodically push it’s latest checkpoint to that bucket
- The checkpoint method will allow for this process to be manually triggered
- On startup the container will pull the most recent version from that s3 bucket if one is available

### Replication

- Only 1 container can be configured as a “leader” and will support writes and checkpoint operations (insertion, deletion, upvote, checkpoint endpoints)
- However multiple containers can run as followers.
- Followers will only support reads
- Followers will periodically poll the s3 bucket for more recent checkpoints, if one is found they will load it and use it to serve queries.
- As long as the single leader constraint is maintained, there can be any number of followers as long as the followers can access the s3 bucket


### Architecture Diagram
<img width="685" height="612" alt="architecture" src="https://github.com/user-attachments/assets/764a7a76-3716-464c-a75f-f2fb363abc86" />
