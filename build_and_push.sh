#!/usr/bin/env bash
# ==============================================================================
# build_and_push.sh
# Automated Docker Build, Version Increment, and Local Registry Deployment
# ==============================================================================
set -euo pipefail

# Configuration Defaults (can be overridden via environment variables)
REGISTRY="${DOCKER_REGISTRY:-192.168.1.104:5000}"
IMAGE_NAME="${DOCKER_IMAGE_NAME:-smakinson/proxyserver}"
PLATFORM="${DOCKER_PLATFORM:-linux/amd64}"
VERSION_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/.version"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ANSI Colors
BOLD=$'\033[1m'
GREEN=$'\033[0;32m'
SKY=$'\033[0;36m'
AMBER=$'\033[0;33m'
ROSE=$'\033[0;31m'
RESET=$'\033[0m'

# Print Help
show_help() {
  cat << EOF
${BOLD}Usage:${RESET} ./build_and_push.sh [OPTIONS] [VERSION]

Builds, tags, and pushes ProxyServer Docker container to the local registry.

${BOLD}Arguments:${RESET}
  [VERSION]           Explicit version to release (e.g. 2.0.1). If omitted,
                      the patch number in .version is automatically incremented.

${BOLD}Options:${RESET}
  --patch             Increment patch version (default: 2.0.0 -> 2.0.1)
  --minor             Increment minor version (e.g. 2.0.0 -> 2.1.0)
  --major             Increment major version (e.g. 2.0.0 -> 3.0.0)
  --skip-frontend     Skip compiling frontend React assets with npm run build
  --skip-git          Skip committing version bump, git tagging, and pushing to GitHub
  --dry-run           Display commands and versions without executing docker or pushing
  -h, --help          Show this help message

${BOLD}Environment Variables:${RESET}
  DOCKER_REGISTRY     Target registry (default: 192.168.1.104:5000)
  DOCKER_IMAGE_NAME   Repository name (default: smakinson/proxyserver)
  DOCKER_PLATFORM     Target architecture (default: linux/amd64)

EOF
}

# 1. Parse Arguments
BUMP_TYPE="patch"
EXPLICIT_VERSION=""
SKIP_FRONTEND=false
SKIP_GIT=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --patch)
      BUMP_TYPE="patch"
      shift
      ;;
    --minor)
      BUMP_TYPE="minor"
      shift
      ;;
    --major)
      BUMP_TYPE="major"
      shift
      ;;
    --skip-frontend)
      SKIP_FRONTEND=true
      shift
      ;;
    --skip-git)
      SKIP_GIT=true
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    -h|--help)
      show_help
      exit 0
      ;;
    *)
      if [[ -z "$EXPLICIT_VERSION" && "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+.*$ ]]; then
        EXPLICIT_VERSION="$1"
        shift
      else
        echo -e "${ROSE}[!] Unknown option or argument: $1${RESET}" >&2
        show_help
        exit 1
      fi
      ;;
  esac
done

# 2. Determine Current and Next Version
if [[ ! -f "$VERSION_FILE" ]]; then
  echo "2.0.0" > "$VERSION_FILE"
fi

CURRENT_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"
if [[ -z "$CURRENT_VERSION" ]]; then
  CURRENT_VERSION="2.0.0"
fi

if [[ -n "$EXPLICIT_VERSION" ]]; then
  NEW_VERSION="$EXPLICIT_VERSION"
else
  # Parse SemVer X.Y.Z
  IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"
  MAJOR="${MAJOR:-2}"
  MINOR="${MINOR:-0}"
  PATCH="${PATCH:-0}"

  # Strip any prerelease suffix from patch number for arithmetic
  PATCH_NUM="${PATCH%%-*}"

  case "$BUMP_TYPE" in
    major)
      NEW_VERSION="$((MAJOR + 1)).0.0"
      ;;
    minor)
      NEW_VERSION="${MAJOR}.$((MINOR + 1)).0"
      ;;
    patch|*)
      NEW_VERSION="${MAJOR}.${MINOR}.$((PATCH_NUM + 1))"
      ;;
  esac
fi

echo -e "${BOLD}${SKY}======================================================${RESET}"
echo -e "${BOLD}${SKY}  ProxyServer Docker Build & Push Pipeline${RESET}"
echo -e "${BOLD}${SKY}======================================================${RESET}"
echo -e "  Current Version:  ${AMBER}${CURRENT_VERSION}${RESET}"
echo -e "  New Version:      ${GREEN}${NEW_VERSION}${RESET}"
echo -e "  Target Registry:  ${SKY}${REGISTRY}/${IMAGE_NAME}${RESET}"
echo -e "  Platform:         ${SKY}${PLATFORM}${RESET}"
echo -e "${BOLD}${SKY}======================================================${RESET}\n"

# 3. Compile React Frontend Assets
if [ "$SKIP_FRONTEND" = false ]; then
  if [ -d "$SCRIPT_DIR/web" ]; then
    echo -e "${BOLD}[1/5] Building React frontend assets...${RESET}"
    if [ "$DRY_RUN" = true ]; then
      echo "  [dry-run] (cd web && npm run build)"
    else
      (cd "$SCRIPT_DIR/web" && npm run build)
      echo -e "${GREEN}✓ Frontend compiled to web/dist/${RESET}\n"
    fi
  fi
else
  echo -e "${AMBER}[1/5] Skipping frontend compilation (--skip-frontend)${RESET}\n"
fi

# 4. Synchronize Version in pkg/config/config.go, web/package.json, and .version
echo -e "${BOLD}[2/5] Synchronizing version to ${NEW_VERSION}...${RESET}"
if [ "$DRY_RUN" = false ]; then
  echo "$NEW_VERSION" > "$VERSION_FILE"

  # Update Version in pkg/config/config.go
  if [[ -f "$SCRIPT_DIR/pkg/config/config.go" ]]; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "s/var Version = \".*\"/var Version = \"${NEW_VERSION}\"/" "$SCRIPT_DIR/pkg/config/config.go" || true
    else
      sed -i "s/var Version = \".*\"/var Version = \"${NEW_VERSION}\"/" "$SCRIPT_DIR/pkg/config/config.go" || true
    fi
  fi

  # Update version in web/package.json
  if [[ -f "$SCRIPT_DIR/web/package.json" ]]; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "s/\"version\": \".*\"/\"version\": \"${NEW_VERSION}\"/" "$SCRIPT_DIR/web/package.json" || true
    else
      sed -i "s/\"version\": \".*\"/\"version\": \"${NEW_VERSION}\"/" "$SCRIPT_DIR/web/package.json" || true
    fi
  fi

  echo -e "${GREEN}✓ Updated .version, pkg/config/config.go, and web/package.json${RESET}\n"
else
  echo "  [dry-run] write ${NEW_VERSION} to .version, pkg/config/config.go, and web/package.json"
fi

# 5. Build Docker Image
echo -e "${BOLD}[3/5] Building Docker container (${PLATFORM})...${RESET}"
CMD_BUILD="docker build --platform ${PLATFORM} --build-arg VERSION=${NEW_VERSION} -t ${IMAGE_NAME}:latest -t ${IMAGE_NAME}:${NEW_VERSION} \"${SCRIPT_DIR}\""

if [ "$DRY_RUN" = true ]; then
  echo "  [dry-run] $CMD_BUILD"
else
  eval "$CMD_BUILD"
  echo -e "${GREEN}✓ Docker build successful${RESET}\n"
fi

# 6. Tag and Push to Registry
REMOTE_VERSIONED="${REGISTRY}/${IMAGE_NAME}:${NEW_VERSION}"
REMOTE_LATEST="${REGISTRY}/${IMAGE_NAME}:latest"

echo -e "${BOLD}[4/5] Tagging and pushing to local registry...${RESET}"
CMD_TAG_VERSION="docker image tag ${IMAGE_NAME}:latest ${REMOTE_VERSIONED}"
CMD_TAG_LATEST="docker image tag ${IMAGE_NAME}:latest ${REMOTE_LATEST}"
CMD_PUSH_VERSION="docker image push ${REMOTE_VERSIONED}"
CMD_PUSH_LATEST="docker image push ${REMOTE_LATEST}"

if [ "$DRY_RUN" = true ]; then
  echo "  [dry-run] $CMD_TAG_VERSION"
  echo "  [dry-run] $CMD_TAG_LATEST"
  echo "  [dry-run] $CMD_PUSH_VERSION"
  echo "  [dry-run] $CMD_PUSH_LATEST"
else
  echo -e "  Tagging ${REMOTE_VERSIONED}..."
  eval "$CMD_TAG_VERSION"
  eval "$CMD_TAG_LATEST"

  echo -e "  Pushing ${REMOTE_VERSIONED}..."
  eval "$CMD_PUSH_VERSION"

  echo -e "  Pushing ${REMOTE_LATEST}..."
  eval "$CMD_PUSH_LATEST"

  echo -e "${GREEN}✓ Pushed to registry successfully${RESET}\n"
fi

# 7. Git Tag and GitHub Release
if [ "$SKIP_GIT" = false ]; then
  echo -e "${BOLD}[5/5] Creating Git release and pushing to GitHub...${RESET}"
  TAG_NAME="v${NEW_VERSION}"
  if [ "$DRY_RUN" = true ]; then
    echo "  [dry-run] git add $VERSION_FILE $SCRIPT_DIR/pkg/config/config.go $SCRIPT_DIR/web/package.json"
    echo "  [dry-run] git commit -m \"chore: release ${TAG_NAME}\""
    echo "  [dry-run] git tag -a \"${TAG_NAME}\" -m \"Release ${TAG_NAME}\""
    echo "  [dry-run] git push origin main"
    echo "  [dry-run] git push origin \"${TAG_NAME}\""
    echo -e "${GREEN}✓ [dry-run] Release tag ${TAG_NAME} prepared${RESET}\n"
  else
    # Stage version changes if any
    git add "$VERSION_FILE" "$SCRIPT_DIR/pkg/config/config.go" "$SCRIPT_DIR/web/package.json" 2>/dev/null || true
    if ! git diff --cached --quiet; then
      git commit -m "chore: release ${TAG_NAME}"
      echo -e "  ${GREEN}✓ Committed release files${RESET}"
    fi

    # Create annotated tag if it does not already exist
    if git rev-parse "$TAG_NAME" >/dev/null 2>&1; then
      echo -e "  ${AMBER}Tag ${TAG_NAME} already exists locally.${RESET}"
    else
      git tag -a "$TAG_NAME" -m "Release ${TAG_NAME}"
      echo -e "  ${GREEN}✓ Created tag ${TAG_NAME}${RESET}"
    fi

    # Push commits to origin main
    echo -e "  Pushing commits to GitHub (origin main)..."
    git push origin main

    # Push tag to origin
    echo -e "  Pushing tag to GitHub (origin ${TAG_NAME})..."
    git push origin "$TAG_NAME"
    echo -e "${GREEN}✓ Pushed tag ${TAG_NAME} to GitHub. Release workflow triggered!${RESET}\n"
  fi
else
  echo -e "${AMBER}[5/5] Skipping Git release tag & push (--skip-git)${RESET}\n"
fi

echo -e "${BOLD}${GREEN}======================================================${RESET}"
echo -e "${BOLD}${GREEN}  Deployment & Release Complete!${RESET}"
echo -e "${BOLD}${GREEN}======================================================${RESET}"
echo -e "  Version: ${BOLD}${NEW_VERSION}${RESET}"
echo -e "  Pull command:"
echo -e "    ${SKY}docker pull ${REMOTE_VERSIONED}${RESET}"
echo -e "  Or latest:"
echo -e "    ${SKY}docker pull ${REMOTE_LATEST}${RESET}"
if [ "$SKIP_GIT" = false ]; then
echo -e "  GitHub Release:"
echo -e "    ${SKY}https://github.com/sebmak/ProxyServer/releases/tag/v${NEW_VERSION}${RESET}"
fi
echo -e "${BOLD}${GREEN}======================================================${RESET}"
