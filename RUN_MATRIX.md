goreleaser release --snapshot --clean --skip=publish

clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/config.Authors.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/config.Base.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/config.MCPMode.multi.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/config.Personas.json



clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.MCPMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.jsonMode.MCPMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.jsonMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.multimodel.MCPMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.multimodel.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.multimodel.jsonMode.MCPMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.multimodel.jsonMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.pipeline.MCPMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.pipeline.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.pipeline.jsonMode.MCPMode.json
clear && dist/agon_windows_amd64_v1/agon.exe chat --config configs/scenarios/config.pipeline.jsonMode.json


