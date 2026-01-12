.
|-- .gitignore
|-- .goreleaser.yml
|-- agon.log
|-- CURLY.md
|-- LICENSE
|-- README.md
|-- SERVICES.md
|-- TESTS.md
|-- TREE.md
|-- cmd/
|   `-- agon/
|       |-- main.go
|       `-- main_test.go
|-- go.mod
|-- go.sum
|-- internal/
|   |-- accuracy/
|   |   |-- accuracy.go
|   |   |-- accuracyBatch.go
|   |   |-- accuracy_integration_test.go
|   |   |-- accuracy_prompts.json
|   |   |-- command.go
|   |   `-- types.go
|   |-- appconfig/
|   |   |-- appconfig.go
|   |   |-- appconfig_test.go
|   |   |-- load_integration_test.go
|   |   |-- parameter_templates.go
|   |   `-- show.go
|   |-- benchmark/
|   |   |-- benchmark.go
|   |   |-- benchmark_integration_test.go
|   |   |-- benchmark_model.go
|   |   |-- benchmark_runtime_test.go
|   |   |-- benchmark_test.go
|   |   |-- command.go
|   |   `-- types.go
|   |-- chat/
|   |   |-- chat.go
|   |   `-- chat_test.go
|   |-- commands/
|   |   |-- accuracy.go
|   |   |-- agon.log
|   |   |-- analyze.go
|   |   |-- analyze_metrics.go
|   |   |-- benchmark.go
|   |   |-- benchmark_accuracy.go
|   |   |-- benchmark_model.go
|   |   |-- benchmark_models.go
|   |   |-- chat.go
|   |   |-- chat_test.go
|   |   |-- command_list.go
|   |   |-- delete.go
|   |   |-- fetch.go
|   |   |-- list.go
|   |   |-- list_commands.go
|   |   |-- models_commands_integration_test.go
|   |   |-- models_delete.go
|   |   |-- models_list.go
|   |   |-- models_metadata_fetch.go
|   |   |-- models_parameters_list.go
|   |   |-- models_parameters_list_test.go
|   |   |-- models_pull.go
|   |   |-- models_show_info.go
|   |   |-- models_sync.go
|   |   |-- models_sync_configs.go
|   |   |-- models_unload.go
|   |   |-- pull.go
|   |   |-- rag.go
|   |   |-- rag_index.go
|   |   |-- rag_preview.go
|   |   |-- root.go
|   |   |-- root_flags_test.go
|   |   |-- root_test.go
|   |   |-- run.go
|   |   |-- show.go
|   |   |-- show_config.go
|   |   |-- sync.go
|   |   `-- unload.go
|   |-- logging/
|   |   |-- logging.go
|   |   `-- logging_test.go
|   |-- metrics/
|   |   |-- aggregator.go
|   |   |-- analyze.go.old
|   |   |-- analyzeMetrics.go
|   |   |-- analyze_entry.go
|   |   |-- analyze_metrics_helpers_test.go
|   |   |-- analyze_metrics_test.go
|   |   |-- benchmark_parse.go
|   |   |-- legacy_types.go
|   |   |-- provider.go
|   |   |-- report.go
|   |   `-- types.go
|   |-- models/
|   |   |-- commands.go
|   |   |-- llama_host.go
|   |   |-- llama_host_test.go
|   |   |-- metadata.go
|   |   |-- metadata_command.go
|   |   |-- metadata_test.go
|   |   |-- models_test.go
|   |   |-- parameters.go
|   |   `-- types.go
|   |-- providerfactory/
|   |   |-- factory.go
|   |   `-- factory_test.go
|   |-- providers/
|   |   |-- provider.go
|   |   |-- llamacpp/
|   |   |   |-- provider.go
|   |   |   |-- provider_helpers_test.go
|   |   |   |-- provider_integration_test.go
|   |   |   `-- provider_test.go
|   |   |-- mcp/
|   |   |   |-- process.go
|   |   |   |-- provider.go
|   |   |   |-- rpc.go
|   |   |   `-- tools.go
|   |   |-- multiplex/
|   |   |   |-- provider.go
|   |   |   `-- provider_test.go
|   |   `-- ollama/
|   |       `-- .gitignore
|   |-- rag/
|   |   |-- chunker.go
|   |   |-- config.go
|   |   |-- embedding.go
|   |   |-- formatter.go
|   |   |-- formatter_test.go
|   |   |-- indexer.go
|   |   |-- preview_command.go
|   |   |-- retriever.go
|   |   |-- retriever_test.go
|   |   `-- types.go
|   |-- tui/
|   |   |-- mcp_status.go
|   |   |-- provider_test.go
|   |   |-- tui.go
|   |   |-- tui_jsonmode_integration_test.go
|   |   |-- tui_multimodel.go
|   |   |-- tui_multimodel_test.go
|   |   |-- tui_pipeline.go
|   |   |-- tui_test.go
|   |   `-- tui_update_view_test.go
|   `-- util/
|       |-- util.go
|       `-- util_test.go
|-- servers/
|   |-- benchmark/
|   |   |-- agon-benchmark.yml
|   |   |-- main.go
|   |   `-- main_test.go
|   `-- mcp/
|       |-- main.go
|       `-- tools/
|           |-- available_tools.go
|           |-- current_time.go
|           |-- current_weather.go
|           |-- current_weather_test.go
|           `-- tools.go


