#!/bin/bash
# Run in AWS CloudShell (us-east-1). Sets up SSH to the Minecraft instance:
#   1. opens port 22 of the instance's security group to one address (yours), as /32;
#   2. puts the public key in the login user's authorized_keys through SSM (the key is appended once, nothing else is
#      changed, sshd is not restarted unless it is not running);
#   3. prints the user name and the host key fingerprint so that the first connection can be checked.
#   bash ssh-setup.sh 181.78.172.86
set -eu
REGION=us-east-1
INSTANCE=i-03ec076c40f788eaa
IP=${1:?usage: bash ssh-setup.sh <your public IP>}
KEY='ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEagpVVJPyYLG2+TGvJpYmXnr1+H5M8g5d3CN/hSQ5XN gocraft-ec2-dan'

SG=$(aws ec2 describe-instances --region $REGION --instance-ids $INSTANCE --query 'Reservations[0].Instances[0].SecurityGroups[].GroupId' --output text)
echo "security group(s): $SG"
for g in $SG; do
	echo "-- rules for port 22 in $g now:"
	aws ec2 describe-security-groups --region $REGION --group-ids $g --query 'SecurityGroups[0].IpPermissions[?FromPort==`22`].IpRanges[].[CidrIp,Description]' --output text || true
done
G=${SG%% *}
if aws ec2 authorize-security-group-ingress --region $REGION --group-id $G --ip-permissions "IpProtocol=tcp,FromPort=22,ToPort=22,IpRanges=[{CidrIp=$IP/32,Description=dan ssh (gocraft)}]" >/dev/null 2>&1; then
	echo "opened 22 to $IP/32 in $G"
else
	echo "(the rule for $IP/32 already exists, or could not be added: check the list above)"
fi

CMD="bash <<'INNER'
set -e
U=\$(ls /home | grep -v -E '^(minecraft|mcbot|ssm-user)\$' | head -1)
[ -n \"\$U\" ] || U=ubuntu
H=/home/\$U
mkdir -p \$H/.ssh && chmod 700 \$H/.ssh
grep -qF 'gocraft-ec2-dan' \$H/.ssh/authorized_keys 2>/dev/null || echo '$KEY' >> \$H/.ssh/authorized_keys
chmod 600 \$H/.ssh/authorized_keys && chown -R \$U:\$U \$H/.ssh
systemctl is-active ssh sshd 2>/dev/null | head -2
echo USER=\$U
for f in /etc/ssh/ssh_host_ed25519_key.pub; do ssh-keygen -lf \$f; done
INNER"
ID=$(aws ssm send-command --region $REGION --instance-ids $INSTANCE --document-name AWS-RunShellScript \
	--parameters "$(python3 -c 'import json,sys;print(json.dumps({"commands":[sys.argv[1]]}))' "$CMD")" --query Command.CommandId --output text)
for i in $(seq 1 24); do
	st=$(aws ssm get-command-invocation --region $REGION --command-id "$ID" --instance-id $INSTANCE --query Status --output text 2>/dev/null || echo Pending)
	case "$st" in Success|Failed|Cancelled|TimedOut) break ;; esac
	sleep 5
done
echo "status: $st"
aws ssm get-command-invocation --region $REGION --command-id "$ID" --instance-id $INSTANCE --query StandardOutputContent --output text
aws ssm get-command-invocation --region $REGION --command-id "$ID" --instance-id $INSTANCE --query StandardErrorContent --output text
