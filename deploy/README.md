# Deploying the Skill Catalog Server

## Local (no cluster needed)

Run the catalog server directly on your machine. Useful for
development and testing before deploying to a cluster.

### Serve from Quay.io (auto-discovery)

```bash
make build
bin/skillctl serve \
  --registry quay.io \
  --namespace skillimage \
  --tls-verify \
  --sync-interval 300s
```

The server auto-detects Quay and discovers all public repos in
the `skillimage` organization. New repos are picked up on the
next sync cycle.

> **Note:** Quay.io defaults new repos to private. If a pushed
> skill doesn't appear after sync, check that the repository
> is set to public in Quay's web UI.

Then in another terminal:

```bash
curl http://localhost:8080/api/v1/skills | jq
curl http://localhost:8080/api/v1/skills/business/document-reviewer | jq
curl -X POST http://localhost:8080/api/v1/sync
```

### Serve from a local OCI registry

Start a local registry (e.g. [zot](https://zotregistry.dev)):

```bash
# Run zot on port 5000
podman run -d -p 5000:5000 ghcr.io/project-zot/zot-linux-amd64:latest

# Push skills to it
bin/skillctl build examples/document-reviewer
bin/skillctl push test/document-reviewer:1.0.0-draft localhost:5000/test/document-reviewer:1.0.0-draft --tls-verify=false

# Serve — local registries support /v2/_catalog, so no --repositories needed
bin/skillctl serve --registry localhost:5000 --tls-verify=false
```

### SQLite database

By default the database is created at `./skillctl.db`. Use `--db`
to change the path, or `:memory:` for a non-persistent in-memory
database:

```bash
bin/skillctl serve --registry quay.io --namespace skillimage --db :memory:
```

## Dev workflow (build, push, deploy)

For iterating on changes against a live OpenShift cluster:

```bash
make deploy
```

This single command builds the image, pushes it, restarts the
deployment (which pulls the new `:latest` via `imagePullPolicy:
Always`), and tails the logs. The full cycle takes about 30 seconds.

If you only want to test locally without pushing to a cluster:

```bash
make build && bin/skillctl serve --registry quay.io --namespace skillimage
```

## OpenShift with Internal Registry

The OpenShift overlay deploys the catalog server configured to index
skills from the internal OpenShift registry.

### Prerequisites

- OpenShift cluster with the internal image registry enabled
- Skills pushed as OCI images to the internal registry
- `oc` CLI logged in with cluster access

### Quick start

```bash
# Create a namespace for the catalog server
oc new-project skill-catalog

# Deploy (uses internal registry by default)
oc apply -k deploy/overlays/openshift

# Verify
oc get pods -l app.kubernetes.io/name=skillctl-catalog
oc logs deploy/skillctl-catalog
```

### Grant access to skill images in other namespaces

The deployment includes a RoleBinding for `system:image-puller` in
the server's namespace. If skills live in a different namespace,
grant pull access there too:

```bash
oc policy add-role-to-user system:image-puller \
  system:serviceaccount:skill-catalog:skillctl-catalog \
  -n <skills-namespace>
```

### Filter by namespace prefix

To index only skills from a specific namespace prefix, patch the
ConfigMap:

```bash
oc patch configmap skillctl-catalog-config --type merge \
  -p '{"data":{"registry-namespace":"team1"}}'
```

### Access from inside the cluster

Other services (RHDH plugins, MLflow, custom frontends) can reach
the server via the Kubernetes Service:

```
http://skillctl-catalog.skill-catalog.svc:8080/api/v1/skills
```

### Access from outside the cluster

The OpenShift overlay creates a TLS-terminated Route:

```bash
# Get the route URL
ROUTE=$(oc get route skillctl-catalog -o jsonpath='{.spec.host}')
curl https://$ROUTE/api/v1/skills
```

### Using Quay.io (auto-discovery)

The catalog server auto-detects Quay registries and uses the
Quay REST API for repository discovery. Set `--namespace` to
the Quay organization name:

```bash
oc patch configmap skillctl-catalog-config --type merge \
  -p '{"data":{"registry-url":"quay.io","registry-namespace":"myorg"}}'
```

> **Important:** Quay.io defaults new repositories to **private**.
> The catalog server can only discover public repositories (unless
> authenticated with an org-admin token). After pushing a new skill
> image, go to **quay.io → Repository Settings → Make Public**, or
> the sync will silently skip it. If a repo you expect is missing,
> check its visibility first.

For self-hosted Quay instances that don't have "quay" in the
hostname, add `--registry-type quay` to force the Quay discovery
adapter.

### Using other external registries (GHCR, Harbor, Docker Hub)

For registries without a discovery adapter, edit the deployment
to add `--repositories` with exact repo names:

```bash
oc set env deploy/skillctl-catalog REGISTRY_REPOSITORIES=myorg/skill-a,myorg/skill-b
oc patch deploy/skillctl-catalog --type json -p '[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--repositories=$(REGISTRY_REPOSITORIES)"}
]'
```

For private external registries that require authentication:

```bash
# Create a pull secret
oc create secret docker-registry skillctl-registry-auth \
  --docker-server=quay.io \
  --docker-username=<user> \
  --docker-password=<token>

# Link the secret to the service account
oc secrets link skillctl-catalog skillctl-registry-auth --for=pull
```

## Plain Kubernetes

Use the base Kustomization and create the ConfigMap manually:

```bash
kubectl create namespace skill-catalog
kubectl -n skill-catalog create configmap skillctl-catalog-config \
  --from-literal=registry-url=<your-registry>
kubectl apply -k deploy/base -n skill-catalog
```

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/skills` | List/search skills |
| GET | `/api/v1/skills/{ns}/{name}` | Skill detail |
| GET | `/api/v1/skills/{ns}/{name}/versions` | All versions |
| GET | `/api/v1/skills/{ns}/{name}/versions/{ver}/content` | SKILL.md |
| POST | `/api/v1/sync` | Trigger re-sync |
| GET | `/healthz` | Health check |
