#!/bin/bash

# --- Configuration ---
AWS_REGION="us-west-1"
ACCOUNT_ID=""
REPO_NAME="thirdai-platform"
IMAGE_NAME="ndb-server"
IMAGE_TAG="1.0.0" 

# --- Derived values ---
ECR_URI="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${REPO_NAME}"

echo "Logging in to Amazon ECR..."
aws ecr get-login-password --region "$AWS_REGION" | docker login --username AWS --password-stdin "${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

if [ $? -ne 0 ]; then
  echo "ECR login failed."
  exit 1
fi

echo "Tagging image..."
docker tag "${IMAGE_NAME}:${IMAGE_TAG}" "${ECR_URI}:${IMAGE_TAG}"

echo "Pushing image to ECR..."
docker push "${ECR_URI}:${IMAGE_TAG}"

if [ $? -eq 0 ]; then
  echo "Image pushed successfully to ${ECR_URI}:${IMAGE_TAG}"
else
  echo "Failed to push image."
  exit 1
fi
