# Deploying the Skill Catalog Server

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

To index only skills from a specific namespace prefix, edit the
ConfigMap:

```bash
oc create configmap skillctl-catalog-config \
  --from-literal=registry-url=image-registry.openshift-image-registry.svc:5000 \
  --from-literal=registry-namespace=team1 \
  --dry-run=client -o yaml | oc apply -f -
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

### Using an external registry (Quay, GHCR, Harbor)

Public registries like Quay.io, GHCR, and Docker Hub do not support
the `/v2/_catalog` API for repository discovery. Use
`--repositories` to specify the exact repository names to sync:

```bash
oc create configmap skillctl-catalog-config \
  --from-literal=registry-url=quay.io \
  --from-literal=registry-repositories=skillimage/business/document-reviewer,skillimage/business/document-summarizer \
  --dry-run=client -o yaml | oc apply -f -
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
