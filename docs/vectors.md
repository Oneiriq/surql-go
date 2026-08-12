# Vector Search

surql-go defines three vector index kinds and two ways to query them. The
choice of index decides where the graph lives; the choice of query operator
decides whether the index is used at all.

## Choosing an index

| kind | graph lives | use when |
|---|---|---|
| `HNSW` | memory | recall latency matters and the working set fits in RAM |
| `DISKANN` | disk | the corpus grows past the memory an HNSW graph would need (SurrealDB 3.2+) |
| `MTREE` | memory | legacy; prefer HNSW for new work |

### HNSW

```go
idx := schema.HnswIndex("embedding_idx", "embedding", 1536, schema.HnswIndexOptions{
    Distance:   schema.HnswDistanceCosine,
    VectorType: schema.MTreeVectorF32,
    EFC:        150,
    M:          12,
})
```

`EFC` and `M` are optional; zero omits the clause and takes the server default.

### DISKANN

```go
idx := schema.DiskAnnIndex("embedding_idx", "embedding", 1536, schema.DiskAnnIndexOptions{
    Distance:   schema.DiskAnnDistanceCosine,
    VectorType: schema.MTreeVectorF16,
    Degree:     48,   // graph out-degree, default 64
    LBuild:     90,   // build-time candidate list, default 100
    Alpha:      "1.5", // pruning slack, default "1.2"
})
```

`DiskAnnDistanceType` is a separate type from `HnswDistanceType` by design:
the engine's DISKANN metric set adds `INNER_PRODUCT` and
`COSINE_NORMALIZED`, and refuses every HNSW metric outside its own four. An
out-of-set metric is therefore unrepresentable rather than merely rejected at
runtime.

Rendered form:

```sql
DEFINE INDEX embedding_idx ON TABLE documents COLUMNS embedding DISKANN
  DIMENSION 1536 DIST COSINE TYPE F16 DEGREE 48 L_BUILD 90 ALPHA 1.5;
```

`DIST`, `TYPE`, `DEGREE`, `L_BUILD`, and `ALPHA` are always spelled, even when
the definition never stated them. The engine fills those defaults in when it
echoes the index back from `INFO FOR TABLE`, so a definition that omitted one
would never compare equal to its own echo and a reconciler would re-apply the
index on every boot. `CanonicalAlpha` renders a whole number bare (`ALPHA 2`)
for the same reason.

## Element types

`MTreeVectorType` is one shared vocabulary and each index kind takes a subset.

| type | MTREE | HNSW | DISKANN |
|---|---|---|---|
| `F64`, `I64`, `I32`, `I16` | yes | yes | no |
| `F32` | yes | yes | yes |
| `F16`, `I8`, `U8` | no | yes | yes |

`IndexDefinition.Validate` refuses the combinations the engine refuses, naming
the accepted set. That matters most for MTREE, where the engine answers a bare
parse error with no explanation of its own.

`F16` halves the memory a graph holds, at a modest cost in recall. Pair it with
a reranking pass over the candidates if precision matters.

## Querying

The second argument of the KNN operator decides the query plan.

```go
// Reaches the index: the engine plans a KnnScan over the HNSW or DISKANN graph.
q, err := query.Query{}.Select(nil).FromTable("documents")
q, err = q.VectorSearchIndexed("embedding", vec, 10, 40)
// SELECT * FROM documents WHERE embedding <|10,40|> [...]

// Exhaustive: the engine plans a KnnTopK over a table scan, comparing every row.
q, err = q.VectorSearch("embedding", vec, 10, query.DistanceCosine, nil)
// SELECT * FROM documents WHERE embedding <|10,COSINE|> [...]
```

An integer in the second position is the exploration factor and reaches the
index. A metric keyword there asks for an exhaustive comparison and the index
serves nothing, which is easy to ship by accident: the statement is valid, the
results are correct, and the cost is a full scan.

`VectorSearchIndexed` takes no metric, because the metric belongs to the index.
Higher `ef` trades speed for recall.

The bare `<|k|>` form of the KTree era is a parse error on SurrealDB 3.x and
neither method emits it.

## Hybrid retrieval

The dense leg above pairs with the lexical leg from a BM25 full-text index. Run
both and fuse the two orders by rank (Reciprocal Rank Fusion) rather than by
score, since the two scales are not comparable. See
[Query Builder](queries.md) for `FullTextSearch` and `SearchScore`.
