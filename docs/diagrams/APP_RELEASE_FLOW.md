# App release flow

What `gorelease [tag]` does when run in a webtyp app (`web/server/main.go` exists). Every branch has a test in
`tests/release_app_test.go`.

```mermaid
flowchart TD
    A["gorelease [tag] in an app"] --> P{"deploy.Scan(rootDir)"}
    P -->|missing or invalid| EP["error from deploy.Scan"]
    P -->|ok| T{"plan.Target == selfupdate.Server?"}
    T -->|no| ET["ErrNotSelfHost"]
    T -->|yes| K{"EnsureSigningKey(store)"}
    K -->|nil store| EK["ErrNoKeyStore"]
    K -->|ok: pub, priv| I["BuildImage(tag, deps)"]
    I -->|error| EI["error from BuildImage"]
    I -->|ok: Image{Name, Tag, Digest}| C["selfupdate.ComposeFile(App{Name, Domain, Repo})"]
    C -->|error| EC["error from ComposeFile"]
    C -->|ok: compose.yaml| S["update.SignManifest(Manifest, priv)"]
    S --> R["gh.CreateRelease(tag, [compose.yaml, manifest.txt, manifest.sig], \"\")<br/>in the app's own repo"]
```
