#!/usr/bin/env python3
"""
Fine-tune sentence-transformers model for Ripls semantic search.

This script fine-tunes all-MiniLM-L6-v2 on domain-specific training data
to improve search relevance for community sharing platform queries.

Usage:
    python train_ripls_embedding.py [--epochs N] [--lr RATE] [--batch-size N]

Output:
    fine_tuned_ripls_model/ - Directory containing the fine-tuned model
"""

import argparse
import json
import sys
from pathlib import Path

from sentence_transformers import SentenceTransformer, InputExample, losses
from sentence_transformers.evaluation import EmbeddingSimilarityEvaluator
from torch.utils.data import DataLoader

# Default configuration
DEFAULT_CONFIG = {
    "base_model": "all-MiniLM-L6-v2",
    "output_path": "fine_tuned_ripls_model",
    "epochs": 5,
    "batch_size": 16,
    "learning_rate": 5e-6,
    "warmup_ratio": 0.1,
    "training_data": "ripls_training_data.json",
    "test_data": "ripls_test_data.json",
}


def load_training_data(path: str) -> list[InputExample]:
    """Load training data and convert to InputExample format."""
    with open(path, "r") as f:
        data = json.load(f)

    examples = []
    for item in data:
        examples.append(InputExample(
            texts=[item["query"], item["item_text"]],
            label=float(item["score"])
        ))

    return examples


def load_evaluation_data(path: str) -> tuple[list[str], list[str], list[float]]:
    """Load test data for evaluation."""
    with open(path, "r") as f:
        data = json.load(f)

    sentences1 = []
    sentences2 = []
    scores = []

    for item in data:
        sentences1.append(item["query"])
        sentences2.append(item["item_text"])
        scores.append(float(item["score"]))

    return sentences1, sentences2, scores


def train(config: dict) -> None:
    """Run the fine-tuning process."""
    print("=" * 60)
    print("Ripls Embedding Model Fine-Tuning")
    print("=" * 60)

    # Print configuration
    print("\nConfiguration:")
    for key, value in config.items():
        print(f"  {key}: {value}")

    # Load base model
    print(f"\nLoading base model: {config['base_model']}...")
    model = SentenceTransformer(config["base_model"])
    print(f"  Model dimensions: {model.get_sentence_embedding_dimension()}")

    # Load training data
    print(f"\nLoading training data from {config['training_data']}...")
    train_examples = load_training_data(config["training_data"])
    print(f"  Loaded {len(train_examples)} training examples")

    # Create data loader
    train_dataloader = DataLoader(
        train_examples,
        shuffle=True,
        batch_size=config["batch_size"]
    )
    print(f"  Batches per epoch: {len(train_dataloader)}")

    # Define loss function
    train_loss = losses.CosineSimilarityLoss(model)

    # Load evaluation data
    print(f"\nLoading evaluation data from {config['test_data']}...")
    eval_sentences1, eval_sentences2, eval_scores = load_evaluation_data(config["test_data"])
    print(f"  Loaded {len(eval_scores)} evaluation examples")

    # Create evaluator
    evaluator = EmbeddingSimilarityEvaluator(
        eval_sentences1,
        eval_sentences2,
        eval_scores,
        name="ripls-eval"
    )

    # Calculate warmup steps
    total_steps = len(train_dataloader) * config["epochs"]
    warmup_steps = int(total_steps * config["warmup_ratio"])
    print(f"\nTraining plan:")
    print(f"  Total steps: {total_steps}")
    print(f"  Warmup steps: {warmup_steps}")

    # Fine-tune the model
    print(f"\nStarting fine-tuning for {config['epochs']} epochs...")
    print("-" * 60)

    model.fit(
        train_objectives=[(train_dataloader, train_loss)],
        evaluator=evaluator,
        epochs=config["epochs"],
        warmup_steps=warmup_steps,
        optimizer_params={"lr": config["learning_rate"]},
        output_path=config["output_path"],
        show_progress_bar=True,
        evaluation_steps=len(train_dataloader),  # Evaluate each epoch
        save_best_model=True,
    )

    print("-" * 60)
    print(f"\nFine-tuning complete!")
    print(f"Model saved to: {config['output_path']}")

    # Final evaluation
    print("\nRunning final evaluation...")
    final_result = evaluator(model)
    # Evaluator returns dict with keys like 'eval_ripls-eval_spearman_cosine'
    if isinstance(final_result, dict):
        spearman_key = [k for k in final_result.keys() if 'spearman' in k.lower()]
        if spearman_key:
            final_score = final_result[spearman_key[0]]
        else:
            final_score = list(final_result.values())[0] if final_result else 0.0
    else:
        final_score = final_result
    print(f"Final evaluation score (Spearman correlation): {final_score:.4f}")

    # Save training config for reference
    config_path = Path(config["output_path"]) / "training_config.json"
    with open(config_path, "w") as f:
        json.dump(config, f, indent=2)
    print(f"Training config saved to: {config_path}")


def main():
    parser = argparse.ArgumentParser(
        description="Fine-tune sentence-transformers for Ripls semantic search"
    )
    parser.add_argument(
        "--epochs", type=int, default=DEFAULT_CONFIG["epochs"],
        help=f"Number of training epochs (default: {DEFAULT_CONFIG['epochs']})"
    )
    parser.add_argument(
        "--lr", type=float, default=DEFAULT_CONFIG["learning_rate"],
        help=f"Learning rate (default: {DEFAULT_CONFIG['learning_rate']})"
    )
    parser.add_argument(
        "--batch-size", type=int, default=DEFAULT_CONFIG["batch_size"],
        help=f"Batch size (default: {DEFAULT_CONFIG['batch_size']})"
    )
    parser.add_argument(
        "--base-model", type=str, default=DEFAULT_CONFIG["base_model"],
        help=f"Base model to fine-tune (default: {DEFAULT_CONFIG['base_model']})"
    )
    parser.add_argument(
        "--output", type=str, default=DEFAULT_CONFIG["output_path"],
        help=f"Output directory for fine-tuned model (default: {DEFAULT_CONFIG['output_path']})"
    )

    args = parser.parse_args()

    # Build config from args
    config = DEFAULT_CONFIG.copy()
    config["epochs"] = args.epochs
    config["learning_rate"] = args.lr
    config["batch_size"] = args.batch_size
    config["base_model"] = args.base_model
    config["output_path"] = args.output

    # Verify data files exist
    for data_file in [config["training_data"], config["test_data"]]:
        if not Path(data_file).exists():
            print(f"Error: Data file not found: {data_file}")
            print("Run generate_training_data.py first to create training data.")
            sys.exit(1)

    train(config)


if __name__ == "__main__":
    main()
