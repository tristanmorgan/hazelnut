# Hazelnut Examples

This directory contains examples for using Hazelnut in different ways.

## Embedding Hazelnut

The `embed` directory shows how to embed Hazelnut into your own Go application.

```go
// Create a new Hazelnut service
hazelnut, err := server.New(ctx, cfg, logger)
if err != nil {
    logger.Error("failed to create hazelnut service", "error", err)
    os.Exit(1)
}

// Start the service in a goroutine
go func() {
    if err := hazelnut.Run(ctx); err != nil {
        logger.Error("hazelnut service error", "error", err)
    }
}()
```

This allows you to run Hazelnut alongside your own application's code.