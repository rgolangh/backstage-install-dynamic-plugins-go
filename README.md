Experimental install script that replaces python script under backstage-showcase.

```bash
go run main.go --dynamic-plugins-file my-plugins.yaml --dynamic-plugins-root target-folder-for-plugins --skip-integrity-check false
```

Example `my-plugins.yaml`:
```yaml
includes:
  - ./dynamic-plugins.default.yaml
plugins:
  # Group: Github
  - package: ./dynamic-plugins/dist/backstage-plugin-scaffolder-backend-module-github-dynamic
  - package: ./dynamic-plugins/dist/backstage-plugin-catalog-backend-module-github-dynamic
    disabled: true
    pluginConfig:
      catalog:
        providers:
          github:
            providerId:
              organization: "${GITHUB_ORG}"

  - disabled: false
    package: "@janus-idp/backstage-plugin-orchestrator-backend-dynamic@1.6.4"
    integrity: >-
      sha512-AbTX5YGJGcpWhlPsLmsysn0TAZLEbSW2lmKu1OuhvP4iI2KQBkF6naN/0iJopEH2s0Itd+k48VN+Q7NeAPu2JA==
    pluginConfig:
      orchestrator:
        dataIndexService:
          url: http://sonataflow-platform-data-index-service
        editor:
          path: https://sandbox.kie.org/swf-chrome-extension/0.32.0
  - disabled: false
    package: "@janus-idp/backstage-plugin-orchestrator@1.8.7"
    integrity: >-
      sha512-cCfXX9y0Fy+l6PfXoZ5ll2vl5buR2GD74TI4XA0uOpH+p2COj7KQg8e8gWqPBMoyvgD6JZiGEUnd/rq6Pn0XMQ==
    pluginConfig:
      dynamicPlugins:
        frontend:
          janus-idp.backstage-plugin-orchestrator:
            appIcons:
            - importName: OrchestratorIcon
              module: OrchestratorPlugin
              name: orchestratorIcon
            dynamicRoutes:
            - importName: OrchestratorPage
              menuItem:
                icon: orchestratorIcon
                text: Orchestrator
              module: OrchestratorPlugin
              path: /orchestrator
```
