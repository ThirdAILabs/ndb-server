an # API Documentation

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
  }
}
```

__Notes__
- All columns intended to be used as metadata must be specified in `"metadata_types"`
- The values in the metadata types map must be the same as supported in the dtype field for constraints (see above).

### Example Response:
```json
{}
```

### Example Usage:
```bash
curl -X POST http://localhost:8000/api/v1/insert \
-H "Content-Type: multipart/form-data" \
-F "file=@example.csv" \
-F "metadata={
  \"filename\": \"example.csv\",
  \"source_id\": \"12345\",
  \"text_columns\": [\"content\"],
  \"metadata_types\": {
    \"author\": \"str\",
    \"year\": \"int\"
  }
}"
```


---

## **3. Delete**
**Description:** Delete documents from the database by their source IDs.

- **Method:** `POST`
- **URL:** `/api/v1/delete`

### Example Request:
```json
{
  "source_ids": ["12345", "67890"]
}
```

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
**Description:** Create a new checkpoint for the database. Only the leader can perform this action.

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
