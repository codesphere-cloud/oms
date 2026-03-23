# Credential for ArgoCD to pull helm charts from Codesphere GHCR
apiVersion: v1
kind: Secret
metadata:
  name: argocd-codesphere-oci-read
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  name: codesphere-charts
  url: ghcr.io/codesphere-cloud/charts
  type: helm
  username: github
  password: "${SECRET_CODESPHERE_OCI_READ}"
  enableOCI: "true"

