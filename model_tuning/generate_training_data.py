#!/usr/bin/env python3
"""
Generate training data for fine-tuning sentence-transformers model.

Creates positive and negative pairs from categories.json for training
a semantic search model optimized for community sharing platform search.

Training data format:
{
    "query": "user search query",
    "item_text": "item name or description",
    "score": 1.0 or 0.0,
    "query_category": "category name",
    "item_category": "category name"
}
"""

import json
import random
from pathlib import Path
from typing import List, Dict, Any, Set, Tuple

# Configuration
POSITIVE_PAIRS_PER_CATEGORY = 30  # Number of positive pairs per category
NEGATIVE_PAIRS_PER_CATEGORY = 30  # Number of negative pairs per category
TEST_SPLIT_RATIO = 0.15  # 15% held out for testing
RANDOM_SEED = 42


def load_categories(path: str = "categories.json") -> Dict[str, Any]:
    """Load category definitions from JSON file."""
    with open(path, "r") as f:
        return json.load(f)


def flatten_categories(categories: Dict[str, Any]) -> List[Dict[str, Any]]:
    """Flatten nested category structure into list of category dicts."""
    flat = []
    for group_name, group_cats in categories.items():
        for cat_name, cat_data in group_cats.items():
            flat.append({
                "name": cat_name,
                "group": group_name,
                "queries": cat_data.get("queries", []),
                "examples": cat_data.get("examples", [])
            })
    return flat


def generate_query_variations(category: Dict[str, Any]) -> List[str]:
    """Generate additional query variations for a category."""
    variations = list(category["queries"])  # Start with existing queries
    cat_name = category["name"]
    cat_lower = cat_name.lower()

    # Add category name as a query
    variations.append(cat_lower)

    # Add various natural language patterns
    patterns = [
        f"I need {cat_lower}",
        f"looking for {cat_lower}",
        f"anyone have {cat_lower}",
        f"can I borrow {cat_lower}",
        f"searching for {cat_lower}",
        f"want to find {cat_lower}",
        f"do you have {cat_lower}",
        f"need to borrow {cat_lower}",
        f"where can I find {cat_lower}",
        f"who has {cat_lower}",
        f"{cat_lower} available",
        f"{cat_lower} to borrow",
        f"{cat_lower} near me",
        f"best {cat_lower}",
        f"cheap {cat_lower}",
    ]
    variations.extend(patterns)

    # Add example-based queries with more variations
    for example in category["examples"]:
        example_lower = example.lower()
        variations.append(f"something like {example_lower}")
        variations.append(f"looking for {example_lower}")
        variations.append(f"need {example_lower}")
        variations.append(example_lower)  # Just the example itself

    # Add word-based variations from category name
    words = cat_lower.split()
    if len(words) > 1:
        # Add partial matches
        for word in words:
            if len(word) > 3:  # Skip short words
                variations.append(word)
                variations.append(f"need {word}")

    return list(set(variations))  # Remove duplicates


def generate_positive_pairs(
    categories: List[Dict[str, Any]],
    pairs_per_category: int,
    seen_pairs: Set[Tuple[str, str]]
) -> List[Dict[str, Any]]:
    """Generate positive (matching) query-item pairs, avoiding duplicates."""
    pairs = []

    for category in categories:
        cat_name = category["name"]
        queries = generate_query_variations(category)
        examples = category["examples"]

        if not queries or not examples:
            continue

        # Generate pairs: query -> example from same category
        generated = 0
        attempts = 0
        max_attempts = pairs_per_category * 10  # Prevent infinite loop

        while generated < pairs_per_category and attempts < max_attempts:
            attempts += 1
            query = random.choice(queries)
            example = random.choice(examples)

            # Check for duplicate
            pair_key = (query.lower(), example.lower())
            if pair_key in seen_pairs:
                continue

            seen_pairs.add(pair_key)
            pairs.append({
                "query": query,
                "item_text": example,
                "score": 1.0,
                "query_category": cat_name,
                "item_category": cat_name
            })
            generated += 1

        # Also add: query -> category name (for classification-style learning)
        cat_pairs_added = 0
        for _ in range(min(5, pairs_per_category // 3)):
            query = random.choice(queries)
            pair_key = (query.lower(), cat_name.lower())
            if pair_key not in seen_pairs:
                seen_pairs.add(pair_key)
                pairs.append({
                    "query": query,
                    "item_text": cat_name,
                    "score": 1.0,
                    "query_category": cat_name,
                    "item_category": cat_name
                })
                cat_pairs_added += 1

    return pairs


def generate_negative_pairs(
    categories: List[Dict[str, Any]],
    pairs_per_category: int,
    seen_pairs: Set[Tuple[str, str]]
) -> List[Dict[str, Any]]:
    """Generate negative (non-matching) query-item pairs, avoiding duplicates."""
    pairs = []

    # Build index of all examples by category
    all_examples = []
    for category in categories:
        for example in category["examples"]:
            all_examples.append({
                "text": example,
                "category": category["name"],
                "group": category["group"]
            })

    for category in categories:
        cat_name = category["name"]
        cat_group = category["group"]
        queries = generate_query_variations(category)

        if not queries:
            continue

        # Get examples from OTHER categories (preferably different groups for harder negatives)
        other_examples = [
            ex for ex in all_examples
            if ex["category"] != cat_name
        ]

        # Prioritize examples from different groups (harder negatives)
        different_group_examples = [
            ex for ex in other_examples
            if ex["group"] != cat_group
        ]

        generated = 0
        attempts = 0
        max_attempts = pairs_per_category * 10

        while generated < pairs_per_category and other_examples and attempts < max_attempts:
            attempts += 1
            query = random.choice(queries)

            # 70% hard negatives (different group), 30% semi-hard (same group, different category)
            if different_group_examples and random.random() < 0.7:
                example_data = random.choice(different_group_examples)
            else:
                example_data = random.choice(other_examples)

            # Check for duplicate
            pair_key = (query.lower(), example_data["text"].lower())
            if pair_key in seen_pairs:
                continue

            seen_pairs.add(pair_key)
            pairs.append({
                "query": query,
                "item_text": example_data["text"],
                "score": 0.0,
                "query_category": cat_name,
                "item_category": example_data["category"]
            })
            generated += 1

    return pairs


def split_train_test(
    data: List[Dict[str, Any]],
    test_ratio: float
) -> tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    """Split data into training and test sets, stratified by category."""
    random.shuffle(data)

    # Group by query_category for stratified split
    by_category: Dict[str, List[Dict[str, Any]]] = {}
    for item in data:
        cat = item["query_category"]
        if cat not in by_category:
            by_category[cat] = []
        by_category[cat].append(item)

    train_data = []
    test_data = []

    for cat, items in by_category.items():
        split_idx = int(len(items) * (1 - test_ratio))
        train_data.extend(items[:split_idx])
        test_data.extend(items[split_idx:])

    random.shuffle(train_data)
    random.shuffle(test_data)

    return train_data, test_data


def print_statistics(data: List[Dict[str, Any]], name: str):
    """Print statistics about generated data."""
    positives = sum(1 for d in data if d["score"] == 1.0)
    negatives = len(data) - positives
    categories = len(set(d["query_category"] for d in data))

    print(f"\n{name} Statistics:")
    print(f"  Total examples: {len(data)}")
    print(f"  Positive pairs: {positives}")
    print(f"  Negative pairs: {negatives}")
    print(f"  Ratio (pos/neg): {positives/negatives:.2f}" if negatives > 0 else "  Ratio: N/A")
    print(f"  Categories covered: {categories}")


def main():
    random.seed(RANDOM_SEED)

    print("Loading categories...")
    categories = load_categories("categories.json")
    flat_categories = flatten_categories(categories)
    print(f"Loaded {len(flat_categories)} categories")

    # Track seen pairs to avoid duplicates
    seen_pairs: Set[Tuple[str, str]] = set()

    print("\nGenerating positive pairs...")
    positive_pairs = generate_positive_pairs(flat_categories, POSITIVE_PAIRS_PER_CATEGORY, seen_pairs)
    print(f"Generated {len(positive_pairs)} positive pairs")

    print("\nGenerating negative pairs...")
    negative_pairs = generate_negative_pairs(flat_categories, NEGATIVE_PAIRS_PER_CATEGORY, seen_pairs)
    print(f"Generated {len(negative_pairs)} negative pairs")

    # Combine and split
    all_data = positive_pairs + negative_pairs
    train_data, test_data = split_train_test(all_data, TEST_SPLIT_RATIO)

    print_statistics(train_data, "Training Data")
    print_statistics(test_data, "Test Data")

    # Save files
    output_dir = Path(".")

    train_path = output_dir / "ripls_training_data.json"
    with open(train_path, "w") as f:
        json.dump(train_data, f, indent=2)
    print(f"\nSaved training data to {train_path}")

    test_path = output_dir / "ripls_test_data.json"
    with open(test_path, "w") as f:
        json.dump(test_data, f, indent=2)
    print(f"Saved test data to {test_path}")

    # Also save category list for reference
    category_list = [cat["name"] for cat in flat_categories]
    with open(output_dir / "category_list.json", "w") as f:
        json.dump(category_list, f, indent=2)
    print(f"Saved category list to category_list.json")

    print("\nDone!")


if __name__ == "__main__":
    main()
