# Agent Plugin examples

`incident-triage/` is a portable Agent Plugins 1.0 package with a root
manifest, one Agent Skill (including a reference file), MCP server
configuration, and sample extension metadata and files.

Build and inspect it with the updated `skillctl` commands:

```bash
skillctl build examples/plugins/incident-triage examples/incident-triage:1.0.0
skillctl list --kind plugin
skillctl inspect examples/incident-triage:1.0.0
```

`skillctl list --kind plugin` shows plugin images already built or pulled into
the local OCI store. Remote registry discovery is not part of this command.

The configured MCP URL uses the reserved `example.com` domain and is a
placeholder, so the package validates but does not connect to a live server.
Replace it with an authorized MCP endpoint before using the plugin with an
agent runtime.

To publish and extract a round trip, push the image to a registry you can
write to, then run:

```bash
skillctl tag examples/incident-triage:1.0.0 quay.io/your-org/incident-triage:1.0.0
skillctl push quay.io/your-org/incident-triage:1.0.0
skillctl pull quay.io/your-org/incident-triage:1.0.0 -o ./plugins/
```
