import requests
from typing import List, Dict, Union, Optional
from typing_extensions import Literal
from pydantic import BaseModel, ValidationError


class AnyOf(BaseModel):
    constraint_type: Literal["AnyOf"] = "AnyOf"
    value: Union[List[int], List[float], List[bool], List[str]]
    dtype: Literal["int", "float", "bool", "str"]


class EqualTo(BaseModel):
    constraint_type: Literal["EqualTo"] = "EqualTo"
    value: Union[int, float, bool, str]
    dtype: Literal["int", "float", "bool", "str"]


class Substring(BaseModel):
    constraint_type: Literal["Substring"] = "Substring"
    value: Union[int, float, bool, str]
    dtype: Literal["int", "float", "bool", "str"]


class LessThan(BaseModel):
    constraint_type: Literal["LessThan"] = "LessThan"
    value: Union[int, float, bool, str]
    dtype: Literal["int", "float", "bool", "str"]


class GreaterThan(BaseModel):
    constraint_type: Literal["GreaterThan"] = "GreaterThan"
    value: Union[int, float, bool, str]
    dtype: Literal["int", "float", "bool", "str"]


class SearchParams(BaseModel):
    query: str
    top_k: int = 10
    constraints: Dict[str, Union[AnyOf, EqualTo, Substring, LessThan, GreaterThan]] = {}


class Reference(BaseModel):
    id: int
    text: str
    source: str
    source_id: str
    metadata: Dict[str, Union[int, float, bool, str]]
    score: float


class SearchResponse(BaseModel):
    query_text: str
    references: List[Reference]


class DocumentMetadata(BaseModel):
    filename: str
    source_id: Optional[str] = None
    text_columns: List[str]
    metadata_types: Dict[str, Literal["int", "float", "bool", "str"]] = {}
    doc_metadata: Dict[str, Union[int, float, bool, str]] = {}


class DeleteParams(BaseModel):
    source_ids: List[str]


class QueryIdPair(BaseModel):
    query_id: int
    reference_id: int


class UpvoteParams(BaseModel):
    query_id_pairs: List[QueryIdPair]


class Source(BaseModel):
    source: str
    source_id: str
    version: int


class NDBClient:
    def __init__(self, base_url: str):
        """
        Initialize the client with the base URL of the server.
        """
        self.base_url = base_url

    def search(self, params: SearchParams) -> SearchResponse:
        """
        Perform a search query.
        """
        url = f"{self.base_url}/api/v1/search"
        try:
            response = requests.post(url, json=params.model_dump())
            response.raise_for_status()
            return SearchResponse(**response.json())
        except ValidationError as e:
            raise ValueError(f"Invalid response format: {e}")
        except requests.RequestException as e:
            raise RuntimeError(f"Search request failed: {e}")

    def insert(self, metadata: DocumentMetadata):
        """
        Insert a document into the database.
        """
        url = f"{self.base_url}/api/v1/insert"
        try:
            with open(metadata.filename, "rb") as file:
                files = {
                    "file": file,
                    "metadata": (None, metadata.model_dump_json(), "application/json"),
                }
                response = requests.post(url, files=files)
                response.raise_for_status()
                return response.json()
        except requests.RequestException as e:
            raise RuntimeError(f"Insert request failed: {e}")

    def delete(self, params: DeleteParams):
        """
        Delete documents by their source IDs.
        """
        url = f"{self.base_url}/api/v1/delete"
        try:
            response = requests.post(url, json=params.model_dump())
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            raise RuntimeError(f"Delete request failed: {e}")

    def upvote(self, params: UpvoteParams):
        """
        Upvote query-reference pairs.
        """
        url = f"{self.base_url}/api/v1/upvote"
        try:
            response = requests.post(url, json=params.model_dump())
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            raise RuntimeError(f"Upvote request failed: {e}")

    def get_sources(self) -> List[Source]:
        """
        Retrieve all sources from the database.
        """
        url = f"{self.base_url}/api/v1/sources"
        try:
            response = requests.get(url)
            response.raise_for_status()
            return [Source(**source) for source in response.json()]
        except ValidationError as e:
            raise ValueError(f"Invalid response format: {e}")
        except requests.RequestException as e:
            raise RuntimeError(f"Get sources request failed: {e}")


# Example usage
if __name__ == "__main__":
    client = NDBClient("http://localhost:3001")

    # Example: Insert
    try:
        metadata = DocumentMetadata(
            filename="test.csv",
            text_columns=["text"],
            metadata_types={"k1": "int", "k2": "str"},
            doc_metadata={},
        )
        insert_response = client.insert(metadata)
        print("Insert Response:", insert_response)
    except Exception as e:
        print("Insert Error:", e)

    # Example: Search
    try:
        search_params = SearchParams(
            query="a b c d e",
            top_k=10,
            constraints={"k1": AnyOf(value=[1, 2, 3], dtype="int")},
        )
        search_results = client.search(search_params)
        print("Search Results:", search_results)
    except Exception as e:
        print("Search Error:", e)

    # Example: Get Sources
    try:
        sources = client.get_sources()
        print("Sources:", sources)
    except Exception as e:
        print("Get Sources Error:", e)

    # Example: Delete
    try:
        delete_params = DeleteParams(source_ids=[sources[0].source_id])
        delete_response = client.delete(delete_params)
        print("Delete Response:", delete_response)
    except Exception as e:
        print("Delete Error:", e)
