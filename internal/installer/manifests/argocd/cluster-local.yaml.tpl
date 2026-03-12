# ArgoCD cluster config for this cluster, where ArgoCD is running
# Additional clusters can be added for ArgoCD to deploy apps to by adding similar secrets
# Read more https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#clusters
apiVersion: v1
kind: Secret
metadata:
  name: "argocd-cluster-dc-${DC_NUMBER}"
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: cluster
stringData:
  name: "dc-${DC_NUMBER}"
  server: https://kubernetes.default.svc # This is a local url because it is the same cluster where ArgoCD is running
  config: |
    {
      "tlsClientConfig":{
        "insecure": false
      }
    }

