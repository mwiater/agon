# Pipeline Mode

Pipeline mode is designed for complex, multi-step workflows by chaining up to four models together in a sequence. In this mode, the output from one model (a "stage") is automatically passed as the input to the next, allowing you to build sophisticated processing chains. For example, you could use the first stage to brainstorm ideas, the second to structure them into an outline, the third to write content, and the fourth to proofread it. This sequential execution is the primary difference from Multimodel mode's parallel nature. It is most useful for tasks that can be broken down into discrete steps, such as data transformation, progressive summarization, or creative writing where each stage builds upon the last. Pipeline mode is mutually exclusive with Multimodel mode but can be combined with `JSONMode` and `MCPMode`.

![Pipeline Mode](.screens/agon_pipelineMode_01.png)

> In Pipeline mode, chain requests together so that the output of one model is the input of the next. See: [configs/config.example.PipelineMode.json](configs/config.example.PipelineMode.json)

## Example Configuration

Start from the provided example file and edit hosts/models as needed:

```bash
cp configs/config.example.PipelineMode.json configs/config.pipeline.json
```

Open `configs/config.pipeline.json` and update host URLs, models, and any parameters you want to test.

## Example Run

```bash
agon chat --config configs/config.pipeline.json
```
