#!/bin/bash
# Run in AWS CloudShell (console, top right), with gocraft-bench.tar.gz and run-on-ec2.sh uploaded (Actions > Upload
# file). It puts the package on the instance through a temporary bucket and SSM, runs the measurement there, prints
# the result and deletes the bucket. Nothing is installed on the instance and the Minecraft service is not touched.
set -eu
REGION=us-east-1
INSTANCE=i-03ec076c40f788eaa
B=gocraft-bench-$(date +%s)-$RANDOM
cd "$HOME"
aws s3 mb "s3://$B" --region $REGION >/dev/null
trap 'aws s3 rb "s3://$B" --force --region $REGION >/dev/null 2>&1 || true' EXIT
aws s3 cp gocraft-bench.tar.gz "s3://$B/" --region $REGION >/dev/null
aws s3 cp run-on-ec2.sh "s3://$B/" --region $REGION >/dev/null
TARURL=$(aws s3 presign "s3://$B/gocraft-bench.tar.gz" --expires-in 600 --region $REGION)
RUNURL=$(aws s3 presign "s3://$B/run-on-ec2.sh" --expires-in 600 --region $REGION)
CMD="bash <<'INNER'
set -e
curl -fsS -o /tmp/gocraft-bench.tar.gz '$TARURL'
curl -fsS -o /tmp/run-on-ec2.sh '$RUNURL'
bash /tmp/run-on-ec2.sh
rm -f /tmp/gocraft-bench.tar.gz /tmp/run-on-ec2.sh
INNER"
ID=$(aws ssm send-command --region $REGION --instance-ids $INSTANCE --document-name AWS-RunShellScript \
	--parameters "$(python3 -c 'import json,sys;print(json.dumps({"commands":[sys.argv[1]],"executionTimeout":["300"]}))' "$CMD")" \
	--query Command.CommandId --output text)
echo "command $ID sent; waiting..."
for i in $(seq 1 60); do
	st=$(aws ssm get-command-invocation --region $REGION --command-id "$ID" --instance-id $INSTANCE --query Status --output text 2>/dev/null || echo Pending)
	case "$st" in Success|Failed|Cancelled|TimedOut) break ;; esac
	sleep 5
done
echo "status: $st"
aws ssm get-command-invocation --region $REGION --command-id "$ID" --instance-id $INSTANCE --query StandardOutputContent --output text
aws ssm get-command-invocation --region $REGION --command-id "$ID" --instance-id $INSTANCE --query StandardErrorContent --output text
