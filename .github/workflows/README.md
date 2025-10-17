# GitHub Actions Workflows

## Test Suite Workflow

**File:** `test.yml`

Comprehensive automated testing workflow that runs:
- Unit tests
- E2E tests with mocked dependencies
- Full integration tests with kind cluster
- Build verification

### Triggers

1. **Manual Trigger** (workflow_dispatch)
   - Go to Actions tab in GitHub
   - Select "Test Suite" workflow
   - Click "Run workflow"
   - Choose branch and run

2. **Pull Requests** to main/master
3. **Pushes** to main/master

### Jobs

#### 1. Unit Tests
- Runs all unit tests in `internal/` directory
- Includes race condition detection
- Fast execution (~30 seconds)

#### 2. E2E Tests (Mocked)
- Runs E2E tests with mock servers
- Tests all platforms (GCP, AWS, Generic)
- No external dependencies required
- Execution time: ~1 minute

#### 3. Integration Tests (Kind)
- Creates real Kubernetes cluster using kind
- Deploys k8s-node-proxy as a pod
- Tests full end-to-end functionality
- Automatically cleans up after test
- Execution time: ~5-10 minutes

#### 4. Build Verification
- Builds the binary
- Uploads artifact for download
- Verifies build succeeds

#### 5. Test Summary
- Aggregates results from all jobs
- Provides pass/fail summary
- Fails if any test job fails

### Artifacts

The workflow uploads the built binary as an artifact:
- **Name:** `k8s-node-proxy`
- **Retention:** 7 days
- **Download:** Available from the workflow run page

### Usage

#### Manual Trigger via GitHub UI

1. Navigate to your repository on GitHub
2. Click **Actions** tab
3. Select **Test Suite** from the left sidebar
4. Click **Run workflow** button (top right)
5. Select branch (default: current branch)
6. Click green **Run workflow** button
7. Watch the workflow execute in real-time

#### Manual Trigger via GitHub CLI

```bash
# Install GitHub CLI (if not already installed)
# macOS: brew install gh
# Linux: https://github.com/cli/cli#installation

# Authenticate
gh auth login

# Run workflow on current branch
gh workflow run test.yml

# Run workflow on specific branch
gh workflow run test.yml --ref feature/my-branch

# List recent workflow runs
gh run list --workflow=test.yml

# Watch a specific run
gh run watch <run-id>

# View run details
gh run view <run-id>
```

### Monitoring

View test results:
1. Go to Actions tab
2. Click on a workflow run
3. Click on individual jobs to see detailed logs
4. Download artifacts from the Summary page

### Troubleshooting

**Integration tests fail with disk space errors:**
- GitHub Actions runners have limited disk space
- Kind clusters can consume significant space
- Workflow includes cleanup steps to mitigate this

**Tests timeout:**
- Default timeout for integration tests: 15 minutes
- Adjust in workflow file if needed:
  ```yaml
  timeout-minutes: 20
  ```

**Build fails:**
- Check Go version compatibility
- Ensure go.mod specifies correct Go version
- Review build logs for specific errors

### Local Testing

Before pushing, test locally with:

```bash
# Run same tests as CI
make test-unit
make test-e2e
make test-e2e-kind
make build
```

### Workflow Optimization

**Caching:**
- Go modules are cached to speed up subsequent runs
- Cache key based on go.sum file hash
- Automatically invalidated when dependencies change

**Parallel Execution:**
- All 4 test jobs run in parallel
- Total workflow time ≈ slowest job (integration tests)
- Estimated total time: ~10-15 minutes

### Future Enhancements

Potential additions:
- Code coverage reporting
- Linting and static analysis
- Security scanning
- Performance benchmarks
- Multi-platform builds (Linux, macOS, Windows)
- Automated releases on tags
