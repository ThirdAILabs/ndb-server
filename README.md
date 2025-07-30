# NDB Server

A simple containerized service for ThirdAI NeuralDB.

## Design 

### Endpoints

The container will expose the following endpoints

- `/search` - runs query
- `/delete` - deletes documents
- `/insert` - inserts documents
- `/upvote` - apples finetuning for future queries
- `/sources` - returns list of documents in NDB
- `/checkpoint` - pushes latest checkpoint (see details on checkpoints below)

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
