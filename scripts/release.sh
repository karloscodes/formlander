#!/bin/bash
# This script is deprecated - releases are now handled by GoReleaser via GitHub Actions
# See: .github/workflows/release-goreleaser.yml
#
# To create a release:
#   git tag -a v1.x.x -m "Release v1.x.x"
#   git push origin v1.x.x
#
# GoReleaser will automatically:
#   - Build multi-arch Docker images
#   - Publish to Docker Hub
#   - Create GitHub Release with changelog

echo "This script is deprecated."
echo ""
echo "Releases are now automated via GitHub Actions."
echo "To create a release:"
echo ""
echo "  git tag -a v1.x.x -m 'Release v1.x.x'"
echo "  git push origin v1.x.x"
echo ""
echo "GitHub Actions will handle the rest."
exit 1

echo -e "${GREEN}🚀 Formlander Release Pipeline${NC}\n"

# Verify we're on main branch
BRANCH=$(git branch --show-current)
if [[ "$BRANCH" != "main" ]]; then
  echo -e "${RED}❌ Releases must run from 'main'. Current branch: $BRANCH${NC}"
  exit 1
fi

# Verify clean working directory
if [[ -n $(git status -s) ]]; then
  echo -e "${RED}❌ Uncommitted changes detected. Commit or stash before releasing.${NC}"
  exit 1
fi

# Run tests
echo -e "${BLUE}🧪 Running test suite...${NC}"
if ! make test; then
  echo -e "${RED}❌ Tests failed${NC}"
  exit 1
fi
echo -e "${GREEN}✓ Tests passed${NC}\n"

# Verify Docker is running
echo -e "${BLUE}� Checking Docker environment...${NC}"
if ! docker info > /dev/null 2>&1; then
  echo -e "${RED}❌ Docker daemon not available${NC}"
  exit 1
fi

# Verify Docker Hub authentication
if ! docker pull hello-world > /dev/null 2>&1; then
  echo -e "${YELLOW}⚠️  Not authenticated to Docker Hub${NC}"
  echo -e "${YELLOW}    Run: docker login${NC}"
  exit 1
fi
echo -e "${GREEN}✓ Docker ready${NC}\n"

# Create or use buildx builder
echo -e "${BLUE}🔧 Setting up buildx builder...${NC}"
if ! docker buildx inspect formlander-builder > /dev/null 2>&1; then
  docker buildx create --name formlander-builder --driver docker-container --use
else
  docker buildx use formlander-builder
fi
echo -e "${GREEN}✓ Builder ready${NC}\n"

# Build multi-arch image
echo -e "${BLUE}🏗️  Building multi-arch image (linux/amd64, linux/arm64)...${NC}"
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --push \
  --tag "$REPO:latest" \
  --tag "$REPO:$SHORT_SHA" \
  --tag "$REPO:$TIMESTAMP" \
  --cache-from type=registry,ref="$REPO:buildcache" \
  --cache-to type=registry,ref="$REPO:buildcache",mode=max \
  --provenance=false \
  --sbom=false \
  .

if [[ $? -ne 0 ]]; then
  echo -e "${RED}❌ Build failed${NC}"
  exit 1
fi
echo -e "${GREEN}✓ Multi-arch build complete${NC}\n"

echo -e "${GREEN}✓ Multi-arch build complete${NC}\n"

# Smoke test the image
echo -e "${BLUE}🧪 Running smoke test...${NC}"
NATIVE_ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')
TEST_IMAGE="$REPO:latest"
TEST_CONTAINER="formlander-smoke-test-$$"
SMOKE_DIR=$(mktemp -d)

cleanup_smoke_test() {
  docker rm -f "$TEST_CONTAINER" > /dev/null 2>&1 || true
  rm -rf "$SMOKE_DIR"
}
trap cleanup_smoke_test EXIT

# Pull the multi-arch image for local platform
docker pull --platform "linux/$NATIVE_ARCH" "$TEST_IMAGE" > /dev/null

docker run -d \
  --name "$TEST_CONTAINER" \
  --platform "linux/$NATIVE_ARCH" \
  -p 18080:8080 \
  -v "$SMOKE_DIR:/app/storage" \
  "$TEST_IMAGE" > /dev/null

# Wait for container to be healthy (max 30 seconds)
echo -e "${YELLOW}  Waiting for container to be healthy...${NC}"
for i in {1..30}; do
  if docker inspect --format='{{.State.Health.Status}}' "$TEST_CONTAINER" 2>/dev/null | grep -q "healthy"; then
    echo -e "  ${GREEN}✓${NC} Container is healthy"
    break
  fi
  if [ $i -eq 30 ]; then
    echo -e "  ${RED}✗${NC} Container failed to become healthy"
    echo -e "\n${RED}Container logs:${NC}"
    docker logs "$TEST_CONTAINER"
    exit 1
  fi
  sleep 1
done

# Test health endpoint directly
echo -e "${YELLOW}  Testing health endpoint...${NC}"
if curl -sf http://localhost:18080/_health > /dev/null; then
  echo -e "  ${GREEN}✓${NC} Health check passed"
else
  echo -e "  ${RED}✗${NC} Health check failed"
  echo -e "\n${RED}Container logs:${NC}"
  docker logs "$TEST_CONTAINER"
  exit 1
fi

# Test admin dashboard accessibility
echo -e "${YELLOW}  Testing admin dashboard...${NC}"
if curl -sf http://localhost:18080/admin/login > /dev/null; then
  echo -e "  ${GREEN}✓${NC} Admin dashboard accessible"
else
  echo -e "  ${RED}✗${NC} Admin dashboard failed"
  echo -e "\n${RED}Container logs:${NC}"
  docker logs "$TEST_CONTAINER"
  exit 1
fi

docker rm -f "$TEST_CONTAINER" > /dev/null 2>&1 || true
echo -e "${GREEN}✓ Smoke test passed${NC}\n"

# Display release summary
echo -e "${GREEN}✅ Release complete!${NC}\n"
echo -e "${BLUE}Published images:${NC}"
echo -e "  • ${YELLOW}$REPO:latest${NC}"
echo -e "  • ${YELLOW}$REPO:$SHORT_SHA${NC}"
echo -e "  • ${YELLOW}$REPO:$TIMESTAMP${NC}"
echo ""
echo -e "${BLUE}Platforms:${NC}"
echo -e "  • linux/amd64"
echo -e "  • linux/arm64"
echo ""
echo -e "${BLUE}Quick start:${NC}"
echo -e "  ${YELLOW}docker run -d -p 8080:8080 -v \$(pwd)/storage:/app/storage $REPO:latest${NC}"
echo ""
