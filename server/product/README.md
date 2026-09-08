# Product

The `product` package enriches gear with manufacturer and retailer specifications fetched from the web. When an AI provider identifies a gear item with high confidence (brand + model number), this package fetches the product page and extracts structured metadata to pre-populate weight, description, and other fields.

## Key files

- `lookup.go` — `Lookup`: the main entry point. `NewLookup` accepts a web fetcher and AI provider; `Lookup.Fetch` runs the retrieval pipeline.
- `search.go` — constructs search queries from a detected brand/model and resolves the most likely product URL.

## When to add code here vs. elsewhere

Web-based product specification retrieval belongs here. The AI-based gear detection that produces the brand/model signal belongs in `server/ai`. The gear service in `server/services/gear` orchestrates both: it calls the AI provider to detect the item, then optionally calls this package to enrich the result.
