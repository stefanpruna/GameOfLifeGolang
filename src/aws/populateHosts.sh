aws ec2 describe-instances | grep PublicIpAddress | grep -o -P "\d+\.\d+\.\d+\.\d+" | grep -v '^10\.' > hosts.txt

sed -i -e 's/^/ubuntu@/' hosts.txt
