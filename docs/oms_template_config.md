## oms template config

Render a config.yaml template using secrets from a vault file

### Synopsis

Render a config.yaml template using secrets from a prod.vault.yaml file.

This command prints the rendered configuration to stdout so templating can be tested without running an installation.

```
oms template config [flags]
```

### Examples

```
# Render config.yaml with secrets from prod.vault.yaml
$ oms template config --config config.yaml --vault prod.vault.yaml --age-key age_key.txt

```

### Options

```
  -k, --age-key string   Path to the age key file used to decrypt the vault (required)
  -c, --config string    Path to the config.yaml template to render (required)
  -h, --help             help for config
  -v, --vault string     Path to the SOPS-encrypted prod.vault.yaml file (required)
```

### SEE ALSO

* [oms template](oms_template.md)	 - Render OMS configuration templates

