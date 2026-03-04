# For ArgoCD to read content of this repo
apiVersion: v1
kind: Secret
metadata:
  name: argocd-codesphere-repos-read
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repo-creds
stringData:
  url: https://github.com/codesphere-cloud
  type: git
  username: github
  password: "${SECRET_CODESPHERE_REPOS_READ}"
