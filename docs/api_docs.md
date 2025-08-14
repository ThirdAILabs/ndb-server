an # API Documentation

## **0. Health**
**Description:** Health check endpoint. Returns status 200 on success.

- **Method:** `GET`
- **URL:** `/api/v1/health`

### Example Usage:
```bash
curl -X GET http://localhost:8000/api/v1/health
```

---

## **1. Search**
**Description:** Search for documents in the NeuralDB based on a query and constraints.

- **Method:** `POST`
- **URL:** `/api/v1/search`

### Example Request:
```json
{
  "query": "example search query",
  "top_k": 5,
  "constraints": {
    "author": {
      "constraint_type": "EqualTo",
      "value": "John Doe",
      "dtype": "str"
    },
    "year": {
      "constraint_type": "AnyOf",
      "value": [2020, 2021],
      "dtype": "int"
    }
  }
}
```

__Notes__
- The `"constraint_type"` field must be one of `"EqualTo"`, `"AnyOf"`, `"Substring"`, `"GreaterThan"`, or `"LessThan"`. 
- The `"dtype"` field can be one of `"str"`, `"int"`, `"float"`, or `"bool"`. 
  - The reason for the dtype field is that json does not always preserve type information. For example integers and floats are reprsented by the same type in json. 
  - The dtype field ensures that the constraints are interpreted the same way they were intended.
- For `"AnyOf"` constraints, the value field must be an array of values. 

### Example Response:
```json
{
  "query_text": "example search query",
  "references": [
    {
      "id": 1,
      "text": "This is a sample document.",
      "source": "example.csv",
      "source_id": "12345",
      "metadata": {
        "author": "John Doe",
        "year": 2021
      },
      "score": 0.95
    }
  ]
}
```

### Example Usage:
```bash
curl -X POST http://localhost:8000/api/v1/search \
-H "Content-Type: application/json" \
-d '{
  "query": "example search query",
  "top_k": 5,
  "constraints": {
    "author": {
      "constraint_type": "EqualTo",
      "value": "John Doe",
      "dtype": "str"
    },
    "year": {
      "constraint_type": "AnyOf",
      "value": [2020, 2021],
      "dtype": "int"
    }
  }
}'
```

---

## **2. Insert**
**Description:** Insert a new document into the database. Only CSV files are supported.

- **Method:** `POST`
- **URL:** `/api/v1/insert`

### Example Request (Multipart Form Data):
- **File:** A CSV file (`example.csv`)
- **Metadata:**
```json
{
  "filename": "example.csv",
  "source_id": "12345",
  "text_columns": ["content"],
  "metadata_types": {
    "author": "str",
    "year": "int"
  },
  "upsert": false
}
```

__Notes__
- The `"source_id"` arg is optional, if not specified a UUID will be generated to identify the source.
- All columns intended to be used as metadata must be specified in `"metadata_types"`
- The values in the metadata types map must be the same as supported in the dtype field for constraints (see above).
- The `"upsert"` arg indicates if old versions of the source should be removed after the insert. This only applies if the `"source_id"` is specified. The default value of this is `false`. Example: if document with id A exists in the ndb with version 1, and a new document with id A is inserted and upsert is true, then it will insert the new document with id A and version 2, then delete version 1 once the insert completes successfully. 

### Example Response:
```json
{}
```

### Example Usage:
```bash
curl -X POST http://localhost:8000/api/v1/insert \
-H "Content-Type: multipart/form-data" \
-F "file=@example.csv" \
-F 'metadata={
  "filename": "example.csv",
  "source_id": "12345",
  "text_columns": ["content"],
  "metadata_types": {
    "author": "str",
    "year": "int"
  }
}'
```


---

## **3. Delete**
**Description:** Delete documents from the database by their source IDs.

- **Method:** `POST`
- **URL:** `/api/v1/delete`

### Example Request:
```json
{
  "source_ids": ["12345", "67890"], 
  "keep_latest_version": false
}
```
__Notes__
- The arg `"keep_latest_version"` indicates if old versions of the sources should be deleted. If true and a give source has versions `[1, 2, 3]` then after the delete it will only have versions `[3]`. The default value of this arg is `false`.

### Example Response:
```json
{}
```

### Example Usage:
```bash
curl -X POST http://localhost:8000/api/v1/delete \
-H "Content-Type: application/json" \
-d '{
  "source_ids": ["12345", "67890"]
}'
```

---

## **4. Upvote**
**Description:** Upvote specific query-document pairs to improve relevance.

- **Method:** `POST`
- **URL:** `/api/v1/upvote`

### Example Request:
```json
{
  "query_id_pairs": [
    {
      "query_text": "example search query",
      "reference_id": 1
    },
    {
      "query_text": "another query",
      "reference_id": 2
    }
  ]
}
```

### Example Response:
```json
{}
```

### Example Usage:
```bash
curl -X POST http://localhost:8000/api/v1/upvote \
-H "Content-Type: application/json" \
-d '{
  "query_id_pairs": [
    {
      "query_text": "example search query",
      "reference_id": 1
    },
    {
      "query_text": "another query",
      "reference_id": 2
    }
  ]
}'
```

---

## **5. Sources**
**Description:** Retrieve a list of all document sources in the database.

- **Method:** `GET`
- **URL:** `/api/v1/sources`

### Example Response:
```json
[
  {
    "source": "example.csv",
    "source_id": "12345",
    "version": 1
  },
  {
    "source": "another_example.csv",
    "source_id": "67890",
    "version": 2
  }
]
```

### Example Usage:
```bash
curl -X GET http://localhost:8000/api/v1/sources
```

---

## **6. Checkpoint**
**Description:** Create a new checkpoint for the database. Only the leader can perform this action. This is done asynchronously, to check the status of a checkpoint use the version endpoint.

- **Method:** `POST`
- **URL:** `/api/v1/checkpoint`

__Notes__
- The `"version"` field will indicate the version of the latest checkpoint. 
- The `"new_checkpoint"` field indicates if a new checkpoint was pushed. If there are no changes to the NeuralDB since the last checkpoint no new checkpoint will be pushed, and this will be false.

### Example Response:
```json
{
  "version": 2,
  "new_checkpoint": true
}
```

### Example Usage:
```bash
curl -X POST http://localhost:8000/api/v1/checkpoint
```

---

## **7. Version**
**Description:** Returns the current checkpoint version of the NeuralDB, as well as the information about the status of the most recent checkpoint push.

- **Method:** `GET`
- **URL:** `/api/v1/version`

__Notes__
- The `"last_checkpoint"` field will be omitted if there has not been a checkpoint, or if it is a follower instance.
- The `"last_checkpoint.version"` field indicates the version of the checkpoint that is being or was created.
- The `"last_checkpoint.complete"` field indicates the checkpoint has been successfully pushed, or failed.
- The `"last_checkpoint.error"` field indicates the error if the checkpoint did not complete successfully. If the error field is not present then the checkpoint can be assumed to have completed successfully.

### Example Response:
```json
{
  "curr_version": 2,
  "last_checkpoint": {
    "version": 3,
    "complete": true,
    "error": "some error message"
  }
}
```

### Example Usage:
```bash
curl -X POST http://localhost:8000/api/v1/checkpoint
```
