"""
Tests to validate training data quality before model fine-tuning.

Run with: pytest tests/test_training_data.py -v
"""

import json
import pytest
from collections import Counter
from pathlib import Path

# Path to data files (relative to model_tuning directory)
DATA_DIR = Path(__file__).parent.parent


@pytest.fixture(scope="module")
def training_data():
    """Load training data."""
    with open(DATA_DIR / "ripls_training_data.json", "r") as f:
        return json.load(f)


@pytest.fixture(scope="module")
def test_data():
    """Load test data."""
    with open(DATA_DIR / "ripls_test_data.json", "r") as f:
        return json.load(f)


@pytest.fixture(scope="module")
def category_list():
    """Load category list."""
    with open(DATA_DIR / "category_list.json", "r") as f:
        return json.load(f)


class TestTrainingDataVolume:
    """Tests for data volume and size."""

    def test_minimum_training_examples(self, training_data):
        """Ensure sufficient training data volume."""
        assert len(training_data) >= 1000, \
            f"Need at least 1000 training examples, got {len(training_data)}"

    def test_minimum_test_examples(self, test_data):
        """Ensure sufficient test data volume."""
        assert len(test_data) >= 100, \
            f"Need at least 100 test examples, got {len(test_data)}"

    def test_test_split_ratio(self, training_data, test_data):
        """Verify test split is reasonable (10-20%)."""
        total = len(training_data) + len(test_data)
        test_ratio = len(test_data) / total
        assert 0.10 <= test_ratio <= 0.25, \
            f"Test ratio {test_ratio:.2%} outside expected range 10-25%"


class TestDataBalance:
    """Tests for positive/negative balance."""

    def test_balanced_training_labels(self, training_data):
        """Verify positive/negative balance is within acceptable range."""
        scores = [d["score"] for d in training_data]
        positives = sum(1 for s in scores if s >= 0.5)
        negatives = len(scores) - positives
        ratio = positives / negatives if negatives > 0 else float("inf")

        assert 0.4 <= ratio <= 2.5, \
            f"Imbalanced training data: {positives} pos, {negatives} neg (ratio={ratio:.2f})"

    def test_balanced_test_labels(self, test_data):
        """Verify test set is also balanced."""
        scores = [d["score"] for d in test_data]
        positives = sum(1 for s in scores if s >= 0.5)
        negatives = len(scores) - positives
        ratio = positives / negatives if negatives > 0 else float("inf")

        assert 0.4 <= ratio <= 2.5, \
            f"Imbalanced test data: {positives} pos, {negatives} neg (ratio={ratio:.2f})"


class TestCategoryCoverage:
    """Tests for category representation."""

    def test_all_categories_in_training(self, training_data, category_list):
        """Ensure all categories have training examples."""
        categories_seen = set(d["query_category"] for d in training_data)
        missing = set(category_list) - categories_seen

        assert len(missing) == 0, f"Missing categories in training: {missing}"

    def test_all_categories_in_test(self, test_data, category_list):
        """Ensure all categories have test examples."""
        categories_seen = set(d["query_category"] for d in test_data)
        missing = set(category_list) - categories_seen

        assert len(missing) == 0, f"Missing categories in test: {missing}"

    def test_minimum_examples_per_category(self, training_data):
        """Each category should have minimum representation."""
        MIN_PER_CATEGORY = 10

        category_counts = Counter(d["query_category"] for d in training_data)
        under_represented = [
            (cat, count) for cat, count in category_counts.items()
            if count < MIN_PER_CATEGORY
        ]

        assert len(under_represented) == 0, \
            f"Categories with < {MIN_PER_CATEGORY} examples: {under_represented}"


class TestDataQuality:
    """Tests for data quality and consistency."""

    def test_no_empty_queries(self, training_data):
        """Verify no empty queries."""
        empty_queries = [
            i for i, d in enumerate(training_data)
            if not d.get("query", "").strip()
        ]
        assert len(empty_queries) == 0, \
            f"Found {len(empty_queries)} empty queries at indices: {empty_queries[:10]}"

    def test_no_empty_item_texts(self, training_data):
        """Verify no empty item texts."""
        empty_items = [
            i for i, d in enumerate(training_data)
            if not d.get("item_text", "").strip()
        ]
        assert len(empty_items) == 0, \
            f"Found {len(empty_items)} empty item_texts at indices: {empty_items[:10]}"

    def test_valid_scores(self, training_data):
        """Verify scores are valid (0.0 or 1.0)."""
        invalid_scores = [
            (i, d["score"]) for i, d in enumerate(training_data)
            if d["score"] not in (0.0, 1.0)
        ]
        assert len(invalid_scores) == 0, \
            f"Found invalid scores: {invalid_scores[:10]}"

    def test_required_fields_present(self, training_data):
        """Verify all required fields are present."""
        required = {"query", "item_text", "score", "query_category", "item_category"}

        for i, d in enumerate(training_data[:100]):  # Check first 100
            missing = required - set(d.keys())
            assert len(missing) == 0, \
                f"Item {i} missing fields: {missing}"


class TestDataUniqueness:
    """Tests for data uniqueness and variety."""

    def test_no_duplicate_pairs(self, training_data):
        """Check for duplicate query-item pairs."""
        pairs = [(d["query"], d["item_text"]) for d in training_data]
        pair_counts = Counter(pairs)
        duplicates = [(p, c) for p, c in pair_counts.items() if c > 1]

        # Allow some duplicates but not excessive
        dup_count = sum(c - 1 for _, c in duplicates)
        dup_ratio = dup_count / len(training_data)

        assert dup_ratio < 0.05, \
            f"Too many duplicates: {dup_count} ({dup_ratio:.1%}). First few: {duplicates[:5]}"

    def test_query_variety(self, training_data):
        """Verify queries have reasonable variety."""
        queries = [d["query"] for d in training_data]
        unique_queries = len(set(queries))
        variety_ratio = unique_queries / len(queries)

        assert variety_ratio > 0.3, \
            f"Low query variety: only {unique_queries} unique out of {len(queries)} ({variety_ratio:.1%})"

    def test_item_variety(self, training_data):
        """Verify items have reasonable variety."""
        items = [d["item_text"] for d in training_data]
        unique_items = len(set(items))
        variety_ratio = unique_items / len(items)

        assert variety_ratio > 0.1, \
            f"Low item variety: only {unique_items} unique out of {len(items)} ({variety_ratio:.1%})"


class TestNegativePairQuality:
    """Tests specific to negative pair quality."""

    def test_negative_pairs_cross_category(self, training_data):
        """Verify negative pairs are from different categories."""
        violations = []
        for i, d in enumerate(training_data):
            if d["score"] == 0.0:
                if d["query_category"] == d["item_category"]:
                    violations.append(i)

        assert len(violations) == 0, \
            f"Found {len(violations)} negative pairs with same category"

    def test_positive_pairs_same_category(self, training_data):
        """Verify positive pairs are from same category."""
        violations = []
        for i, d in enumerate(training_data):
            if d["score"] == 1.0:
                if d["query_category"] != d["item_category"]:
                    violations.append((i, d["query_category"], d["item_category"]))

        assert len(violations) == 0, \
            f"Found {len(violations)} positive pairs with different categories: {violations[:5]}"


class TestQueryDistribution:
    """Tests for query length and distribution."""

    def test_query_length_distribution(self, training_data):
        """Verify queries have realistic length distribution."""
        lengths = [len(d["query"].split()) for d in training_data]
        avg_len = sum(lengths) / len(lengths)

        assert 2 <= avg_len <= 15, \
            f"Average query length {avg_len:.1f} words seems unrealistic"

    def test_no_extremely_long_queries(self, training_data):
        """Verify no extremely long queries that might cause issues."""
        MAX_WORDS = 50
        long_queries = [
            (i, len(d["query"].split()), d["query"][:50])
            for i, d in enumerate(training_data)
            if len(d["query"].split()) > MAX_WORDS
        ]

        assert len(long_queries) == 0, \
            f"Found {len(long_queries)} queries with > {MAX_WORDS} words"

    def test_no_extremely_short_queries(self, training_data):
        """Most queries should have at least 2 words."""
        MIN_WORDS = 1
        short_count = sum(
            1 for d in training_data
            if len(d["query"].split()) < MIN_WORDS
        )
        short_ratio = short_count / len(training_data)

        assert short_ratio < 0.1, \
            f"Too many very short queries: {short_count} ({short_ratio:.1%})"
