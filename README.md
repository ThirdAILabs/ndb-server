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
    	S3 bucket name for checkpoints, optional if not using no checkpoints will be pushed
  -s3-region string
    	S3 region for checkpoints, required if s3-bucket is specified
  -ckpt-interval string
    	Interval for checkpoints (e.g., 5m, 1h5m) (default "1h")
  -ckpt-dir string
    	Local directory to store checkpoints (default "./checkpoints")
  -max-ckpts int
    	Maximum number of checkpoints to keep in S3 (default 10)
  -port int
    	Port to run the server on (default 80)
```
### Running with Docker
1. Build the docker image:
```bash
docker build -t ndb-server -f DockerfileAmd64 .
```
Note: There are two dockerfiles available to use. The difference is that in DockerfileAmd64 a distroless debian image is used for the release stage, which reduces the image size from ~800Mb to ~50Mb. However it requires copying various libraries like libc, libgcc, etc from the build stage which uses the official go image. The paths of these libraries is specific to the architecture, and thus thus dockerfile only works for amd64 machines. The same image could be adapted to work on arm64, it would just require updating these paths.  
2. Run the image: 
```bash
docker run --rm ndb-server --leader --s3-bucket <s3 bucket>
```

## Design 

### Endpoints

The container will expose the following endpoints:

- `/api/v1/search` - runs query
- `/api/v1/delete` - deletes documents
- `/api/v1/insert` - inserts documents
- `/api/v1/upvote` - apples finetuning for future queries
- `/api/v1/sources` - returns list of documents in NDB
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
