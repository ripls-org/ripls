"""
Tests to validate ONNX export produces identical results to PyTorch model.
Critical for ensuring Go inference will match Python training results.

Run with: pytest tests/test_onnx_export.py -v
"""

import pytest
import numpy as np
from pathlib import Path

# These will be imported after checking availability
ort = None
SentenceTransformer = None

# Model paths
FINE_TUNED_MODEL = "fine_tuned_ripls_model"
ONNX_MODEL_PATH = "ripls_embedding.onnx"
TOKENIZER_PATH = "ripls_embedding_tokenizer"

# Test inputs covering edge cases
TEST_INPUTS = [
    "camping tent",
    "baby stroller for jogging in the park with my toddler",
    "drill",
    "a" * 500,  # Long input (will be truncated)
    "cafe resume naive",  # ASCII approximation of unicode
    "test with   multiple   spaces",
    "UPPERCASE AND lowercase MiXeD",
    "REI half dome tent",
    "DeWalt cordless power drill",
    "Garden zucchini surplus",
]

# Maximum allowed difference between PyTorch and ONNX outputs
MAX_EMBEDDING_DIFF = 1e-4


@pytest.fixture(scope="module")
def dependencies_available():
    """Check if required dependencies are available."""
    global ort, SentenceTransformer
    try:
        import onnxruntime as _ort
        from sentence_transformers import SentenceTransformer as ST
        ort = _ort
        SentenceTransformer = ST
        return True
    except ImportError as e:
        pytest.skip(f"Required dependency not installed: {e}")
        return False


@pytest.fixture(scope="module")
def onnx_model_exists():
    """Check if ONNX model has been exported."""
    if not Path(ONNX_MODEL_PATH).exists():
        pytest.skip(f"ONNX model not found at {ONNX_MODEL_PATH}. Run export_to_onnx.py first.")
    return True


@pytest.fixture(scope="module")
def pytorch_model(dependencies_available):
    """Load PyTorch model."""
    model_path = Path(__file__).parent.parent / FINE_TUNED_MODEL
    if not model_path.exists():
        pytest.skip(f"Fine-tuned model not found at {model_path}")
    return SentenceTransformer(str(model_path))


@pytest.fixture(scope="module")
def onnx_session(dependencies_available, onnx_model_exists):
    """Load ONNX model session."""
    onnx_path = Path(__file__).parent.parent / ONNX_MODEL_PATH
    return ort.InferenceSession(str(onnx_path))


@pytest.fixture(scope="module")
def tokenizer(pytorch_model):
    """Get tokenizer from PyTorch model."""
    return pytorch_model.tokenizer


def mean_pool(hidden_states: np.ndarray, attention_mask: np.ndarray) -> np.ndarray:
    """Mean pooling matching sentence-transformers implementation."""
    input_mask_expanded = np.broadcast_to(
        attention_mask[:, :, np.newaxis],
        hidden_states.shape
    ).astype(float)

    sum_embeddings = np.sum(hidden_states * input_mask_expanded, axis=1)
    sum_mask = np.clip(input_mask_expanded.sum(axis=1), a_min=1e-9, a_max=None)

    return sum_embeddings / sum_mask


def normalize(embeddings: np.ndarray) -> np.ndarray:
    """L2 normalize embeddings."""
    norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
    return embeddings / np.clip(norms, a_min=1e-9, a_max=None)


def get_onnx_embedding(session, tokenizer, text: str, max_length: int = 128) -> np.ndarray:
    """Get embedding from ONNX model with mean pooling and normalization."""
    inputs = tokenizer(
        text,
        return_tensors="np",
        padding=True,
        truncation=True,
        max_length=max_length
    )

    outputs = session.run(
        None,
        {
            'input_ids': inputs['input_ids'].astype(np.int64),
            'attention_mask': inputs['attention_mask'].astype(np.int64)
        }
    )

    # Apply mean pooling
    hidden_states = outputs[0]
    pooled = mean_pool(hidden_states, inputs['attention_mask'])

    # Normalize
    normalized = normalize(pooled)

    return normalized[0]  # Return single embedding


class TestONNXExportBasics:
    """Basic tests for ONNX export."""

    def test_onnx_file_exists(self, onnx_model_exists):
        """Verify ONNX file was created."""
        onnx_path = Path(__file__).parent.parent / ONNX_MODEL_PATH
        assert onnx_path.exists()

    def test_onnx_file_size_reasonable(self, onnx_model_exists):
        """ONNX model should be reasonable size (~80-100MB)."""
        onnx_path = Path(__file__).parent.parent / ONNX_MODEL_PATH
        size_mb = onnx_path.stat().st_size / (1024 * 1024)
        assert 50 <= size_mb <= 150, f"ONNX file size {size_mb:.1f}MB seems wrong"

    def test_tokenizer_files_exist(self, onnx_model_exists):
        """Verify tokenizer files were exported."""
        tokenizer_dir = Path(__file__).parent.parent / TOKENIZER_PATH
        if not tokenizer_dir.exists():
            pytest.skip("Tokenizer directory not found")

        required_files = ["tokenizer.json", "vocab.txt"]
        for filename in required_files:
            assert (tokenizer_dir / filename).exists(), f"Missing {filename}"


class TestONNXOutputShape:
    """Test ONNX model output shapes."""

    def test_output_shape_single(self, onnx_session, tokenizer):
        """Single input should produce correct shape."""
        embedding = get_onnx_embedding(onnx_session, tokenizer, "test query")
        assert embedding.shape == (384,), f"Expected (384,), got {embedding.shape}"

    def test_output_normalized(self, onnx_session, tokenizer):
        """Output embeddings should be L2 normalized."""
        for text in TEST_INPUTS[:5]:
            embedding = get_onnx_embedding(onnx_session, tokenizer, text)
            norm = np.linalg.norm(embedding)
            assert 0.99 <= norm <= 1.01, f"Embedding norm {norm:.4f} not normalized"


class TestONNXPyTorchEquivalence:
    """Test that ONNX output matches PyTorch output."""

    @pytest.mark.parametrize("text", TEST_INPUTS)
    def test_embedding_equivalence(self, pytorch_model, onnx_session, tokenizer, text):
        """ONNX embedding should match PyTorch embedding."""
        # Get PyTorch embedding
        pytorch_emb = pytorch_model.encode(text)

        # Get ONNX embedding
        onnx_emb = get_onnx_embedding(onnx_session, tokenizer, text)

        # Compare
        max_diff = np.max(np.abs(pytorch_emb - onnx_emb))
        assert max_diff < MAX_EMBEDDING_DIFF, \
            f"Max diff {max_diff:.6f} exceeds threshold for '{text[:30]}...'"

    def test_similarity_preservation(self, pytorch_model, onnx_session, tokenizer):
        """Similarity scores should be preserved between PyTorch and ONNX."""
        query = "camping tent"
        items = ["REI dome tent", "baby stroller", "power drill"]

        # Get PyTorch similarities
        query_pt = pytorch_model.encode(query)
        items_pt = pytorch_model.encode(items)
        pt_sims = [np.dot(query_pt, item) for item in items_pt]

        # Get ONNX similarities
        query_onnx = get_onnx_embedding(onnx_session, tokenizer, query)
        items_onnx = [get_onnx_embedding(onnx_session, tokenizer, item) for item in items]
        onnx_sims = [np.dot(query_onnx, item) for item in items_onnx]

        # Compare
        for i, (pt_sim, onnx_sim) in enumerate(zip(pt_sims, onnx_sims)):
            diff = abs(pt_sim - onnx_sim)
            assert diff < MAX_EMBEDDING_DIFF, \
                f"Similarity diff {diff:.6f} for '{items[i]}'"

    def test_ranking_preserved(self, pytorch_model, onnx_session, tokenizer):
        """Item rankings should be identical between PyTorch and ONNX."""
        query = "baby stroller for jogging"
        items = [
            "BOB jogging stroller",  # Should rank 1
            "Graco car seat",         # Should rank 2-ish
            "DeWalt power drill",     # Should rank last
        ]

        # Get rankings from PyTorch
        query_pt = pytorch_model.encode(query)
        items_pt = pytorch_model.encode(items)
        pt_scores = [np.dot(query_pt, item) for item in items_pt]
        pt_ranking = sorted(range(len(items)), key=lambda i: pt_scores[i], reverse=True)

        # Get rankings from ONNX
        query_onnx = get_onnx_embedding(onnx_session, tokenizer, query)
        items_onnx = [get_onnx_embedding(onnx_session, tokenizer, item) for item in items]
        onnx_scores = [np.dot(query_onnx, item) for item in items_onnx]
        onnx_ranking = sorted(range(len(items)), key=lambda i: onnx_scores[i], reverse=True)

        assert pt_ranking == onnx_ranking, \
            f"Rankings differ: PyTorch={pt_ranking}, ONNX={onnx_ranking}"


class TestONNXEdgeCases:
    """Test ONNX handles edge cases correctly."""

    def test_long_input_truncation(self, onnx_session, tokenizer):
        """Long inputs should be truncated without error."""
        long_text = "word " * 1000  # Way over max length
        embedding = get_onnx_embedding(onnx_session, tokenizer, long_text, max_length=128)
        assert embedding.shape == (384,)
        assert not np.any(np.isnan(embedding))

    def test_short_input(self, onnx_session, tokenizer):
        """Very short inputs should work."""
        embedding = get_onnx_embedding(onnx_session, tokenizer, "a")
        assert embedding.shape == (384,)
        assert not np.any(np.isnan(embedding))

    def test_special_characters(self, onnx_session, tokenizer):
        """Inputs with special characters should work."""
        texts = [
            "test@email.com",
            "price: $100",
            "50% off!",
            "item #123",
            "2x4 lumber",
        ]
        for text in texts:
            embedding = get_onnx_embedding(onnx_session, tokenizer, text)
            assert embedding.shape == (384,)
            assert not np.any(np.isnan(embedding)), f"NaN in embedding for '{text}'"

    def test_deterministic(self, onnx_session, tokenizer):
        """Same input should produce identical output."""
        text = "test query for determinism"
        emb1 = get_onnx_embedding(onnx_session, tokenizer, text)
        emb2 = get_onnx_embedding(onnx_session, tokenizer, text)
        assert np.allclose(emb1, emb2), "ONNX output not deterministic"
