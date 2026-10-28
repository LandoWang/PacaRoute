# syntax=docker/dockerfile:1
# ---------------------------------------------------------------------------
# copilot-bridge — container image for running the bridge on a cloud host
# (Azure Container Instances etc.). Multi-stage: build the Native-AOT linux-x64
# binary in the .NET 10 SDK image, then copy the self-contained ELF into a slim
# runtime image that carries only libc + the few shared libs AOT links against.
#
# Build (from the repo root, NOT this folder — the build context must be the
# repo so the SDK can restore the whole project):
#   az acr build -r <registry> -t copilot-bridge:latest -f deploy/Dockerfile .
#   # or locally:  docker build -f deploy/Dockerfile -t copilot-bridge:latest .
# ---------------------------------------------------------------------------

# --- Stage 1: AOT publish --------------------------------------------------
# The .NET SDK image does NOT ship a C toolchain, and the ILC native-link step
# shells out to clang + needs zlib headers. Install them (this is exactly the
# aka.ms/nativeaot-prerequisites set for Debian) before publishing.
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
RUN apt-get update \
    && apt-get install -y --no-install-recommends clang zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src

# Restore first (cache-friendly): copy only what the restore graph needs.
COPY global.json Directory.Build.props Directory.Packages.props ./
COPY src/CopilotBridge.Cli/CopilotBridge.Cli.csproj src/CopilotBridge.Cli/
RUN dotnet restore src/CopilotBridge.Cli/CopilotBridge.Cli.csproj -r linux-x64

# Now the sources and publish the single-file AOT binary.
COPY . .
RUN dotnet publish src/CopilotBridge.Cli/CopilotBridge.Cli.csproj \
        -c Release -r linux-x64 \
        --no-restore \
        -o /app

# --- Stage 2: runtime ------------------------------------------------------
# runtime-deps (not aspnet/runtime): the AOT binary has no .NET runtime
# dependency, it just needs the native base libs + ICU-less globalization
# (InvariantGlobalization=true is set in the csproj). dbus is pulled in for
# dbus-uuidgen, used by the entrypoint to seed /etc/machine-id — the token
# store's key derivation reads it and refuses to start if it is missing.
FROM mcr.microsoft.com/dotnet/runtime-deps:10.0 AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends dbus \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /app/copilot-bridge   ./copilot-bridge
COPY --from=build /app/appsettings.json ./appsettings.json
COPY deploy/entrypoint.sh               /usr/local/bin/entrypoint.sh
RUN chmod +x ./copilot-bridge /usr/local/bin/entrypoint.sh

# Bind to all interfaces inside the container (the default is localhost, which
# nothing outside the container could reach). Access control is expected to be
# supplied by the surrounding network — the bridge itself has no inbound auth.
ENV Server__Host=0.0.0.0 \
    Server__Port=8765

EXPOSE 8765
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["serve"]
