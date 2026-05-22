# Generic ArgoCD repository secret
apiVersion: v1
kind: Secret
metadata:
  name: "${SECRET_NAME}"
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: ${SECRET_TYPE}
stringData:
  type: ${REPO_TYPE}
  url: ${REPO_URL}
  name: ${REPO_DISPLAY_NAME}
  username: ${USERNAME}
  password: ${PASSWORD}
  enableOCI: "${ENABLE_OCI}"
