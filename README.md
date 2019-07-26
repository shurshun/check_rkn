# Example of usage the go-app helm chart from my repository

`helm repo add shurshun https://shurshun.github.com/helm-charts`

`helm upgrade --install infra-check-rkn shurshun/go-app --version 0.1.53 \
    --namespace infra \
    -f .helm/values.yaml \
    -f .helm/stages/dev/values.yaml \
    --set-string image.repository=shurshun/check_rkn \
    --set-string image.tag=latest \
    --set-string nameOverride=check-rkn \
    --set-string fullnameOverride=check-rkn`

# App usage

`curl -X POST -d '["8.8.8.8", "8.8.4.4"]' http://api.example.com/v1/rkn/check`

`{"34.246.38.204":true,"47.91.106.69":true}`