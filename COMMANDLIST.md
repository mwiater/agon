Commands and Subcommands:
  agon                          agon â€” terminal-first companion for multi-host llama.cpp workflows
    agon analyze                Analyze benchmark outputs
      agon analyze metrics      Generate metric analysis & report from benchmark JSON
      agon benchmark              Group commands for running benchmarks
      agon benchmark accuracy   Run accuracy batch workflows
      agon benchmark model      Run a single benchmark against a benchmark server endpoint
    agon chat                   Start a chat session
    agon delete                 Group commands for deleting resources
      agon delete models        Delete all models not in the configuration file
    agon fetch                  Group commands for fetching resources
      agon fetch modelmetadata  Fetch model metadata from configured hosts
    agon help                   Help about any command
    agon list                   Group commands for listing resources
      agon list commands        List all commands and subcommands in two columns
      agon list models          List all models on each node
    agon pull                   Group commands for pulling resources
      agon pull models          Pull all models from the configuration file
    agon rag                    RAG utilities
      agon rag index            Build the RAG JSONL index
      agon rag preview          Preview RAG retrieval and context assembly
    agon show                   Group commands for displaying resources
      agon show config          Show config settings
    agon sync                   Group commands for syncing resources
      agon sync configs         
      agon sync models          Sync all models from the configuration file
    agon unload                 Group commands for unloading resources
      agon unload models        Unload all loaded models on each host
