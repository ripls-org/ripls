#!/usr/bin/env python3
"""
Export fine-tuned sentence-transformers model to ONNX format.

This exports:
1. ripls_embedding.onnx - The transformer model
2. ripls_tokenizer/ - Tokenizer files for Go integration

Usage:
    python export_to_onnx.py [--model PATH] [--output NAME]
"""

import argparse
import os
import shutil
from pathlib import Path

import onnx
import torch
from sentence_transformers import SentenceTransformer


def export_model(model_path: str, output_name: str) -> None:
    """Export model to ONNX format."""
    print("=" * 60)
    print("ONNX Model Export")
    print("=" * 60)

    # Load fine-tuned model on CPU (required for ONNX export)
    print(f"\nLoading model from: {model_path}")
    model = SentenceTransformer(model_path, device="cpu")

    # Get components and ensure on CPU
    transformer = model[0].auto_model.cpu()
    tokenizer = model.tokenizer

    print(f"Model loaded: {model.get_sentence_embedding_dimension()} dimensions")

    # Create dummy input for tracing
    dummy_text = "sample text for export tracing"
    dummy_input = tokenizer(
        dummy_text,
        return_tensors="pt",
        padding=True,
        truncation=True,
        max_length=128
    )

    # Export to ONNX
    onnx_path = f"{output_name}.onnx"
    print(f"\nExporting model to: {onnx_path}")

    # Put model in eval mode
    transformer.eval()

    torch.onnx.export(
        transformer,
        (dummy_input['input_ids'], dummy_input['attention_mask']),
        onnx_path,
        input_names=['input_ids', 'attention_mask'],
        output_names=['last_hidden_state'],
        dynamic_axes={
            'input_ids': {0: 'batch_size', 1: 'sequence'},
            'attention_mask': {0: 'batch_size', 1: 'sequence'},
            'last_hidden_state': {0: 'batch_size', 1: 'sequence'}
        },
        opset_version=18,
        do_constant_folding=True,
    )

    # Check if external data file was created
    external_data_path = f"{onnx_path}.data"
    if Path(external_data_path).exists():
        print("Converting external data to single file...")
        # Load model with external data
        model_onnx = onnx.load(onnx_path, load_external_data=True)
        # Save as single file
        onnx.save(model_onnx, onnx_path, save_as_external_data=False)
        # Remove external data file
        os.remove(external_data_path)
        print("External data embedded into ONNX file")

    # Check file size
    onnx_size = Path(onnx_path).stat().st_size / (1024 * 1024)
    print(f"ONNX model size: {onnx_size:.1f} MB")

    # Export tokenizer
    tokenizer_path = f"{output_name}_tokenizer"
    print(f"\nExporting tokenizer to: {tokenizer_path}")

    # Remove existing directory if present
    if Path(tokenizer_path).exists():
        shutil.rmtree(tokenizer_path)

    tokenizer.save_pretrained(tokenizer_path)

    # List exported tokenizer files
    print("Tokenizer files:")
    for f in sorted(Path(tokenizer_path).iterdir()):
        size = f.stat().st_size
        print(f"  {f.name}: {size:,} bytes")

    print("\n" + "=" * 60)
    print("Export complete!")
    print("=" * 60)
    print(f"\nFiles for deployment:")
    print(f"  - {onnx_path} (model)")
    print(f"  - {tokenizer_path}/ (tokenizer)")
    print("\nNext steps:")
    print("  1. Run ONNX validation tests: pytest tests/test_onnx_export.py -v")
    print("  2. Copy to server/ai/models/ for Go integration")


def main():
    parser = argparse.ArgumentParser(
        description="Export sentence-transformers model to ONNX"
    )
    parser.add_argument(
        "--model", type=str, default="fine_tuned_ripls_model",
        help="Path to fine-tuned model directory"
    )
    parser.add_argument(
        "--output", type=str, default="ripls_embedding",
        help="Output name prefix (will create NAME.onnx and NAME_tokenizer/)"
    )

    args = parser.parse_args()

    # Verify model exists
    if not Path(args.model).exists():
        print(f"Error: Model not found at {args.model}")
        print("Run train_ripls_embedding.py first to create the fine-tuned model.")
        return 1

    export_model(args.model, args.output)
    return 0


if __name__ == "__main__":
    exit(main())
