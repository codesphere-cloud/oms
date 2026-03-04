# ArgoCD
apiVersion: v1
kind: Namespace
metadata:
  name: argocd
---
apiVersion: v1
stringData:
  admin.password: "${ARGOCD_ADMIN_PASSWORD_HASH}"
kind: Secret
metadata:
  labels:
    app.kubernetes.io/name: argocd-secret
    app.kubernetes.io/part-of: argocd
  name: argocd-secret
  namespace: argocd
type: Opaque
