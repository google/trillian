variable "WHITELIST_CIDR" {
  description="Your IP block to whitelist access from"
}
variable "DB_PASSWORD" { }

provider "aws" {
  region     = "us-west-2"
}

/* The Database */

resource "aws_rds_cluster" "trillian" {
  cluster_identifier      = "trillian"
  database_name           = "test"
  master_username         = "root"
  master_password         = "${var.DB_PASSWORD}"
  skip_final_snapshot     = true
  port                    = 3306
  vpc_security_group_ids  = ["${aws_security_group.trillian_db.id}"]
  availability_zones      = ["us-west-2a", "us-west-2b", "us-west-2c"]
  storage_encrypted       = true
  apply_immediately       = true

}

resource "aws_rds_cluster_instance" "cluster_instances" {
  count               = 2
  identifier          = "trillian-${count.index}"
  cluster_identifier  = "${aws_rds_cluster.trillian.id}"
  instance_class      = "db.r3.large"
  publicly_accessible = true
  apply_immediately   = true
}

resource "aws_security_group" "trillian_db" {
  name        = "trillian-db"
  description = "Allow MySQL from Trillian and Development CIDR"

  ingress {
    from_port   = 3306
    to_port     = 3306
    protocol    = "tcp"
    cidr_blocks = ["${var.WHITELIST_CIDR}"]
  }

  ingress {
    from_port   = 3306
    to_port     = 3306
    protocol    = "tcp"
    security_groups = ["${aws_security_group.trillian.id}"]
  }

  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    cidr_blocks     = ["0.0.0.0/0"]
  }
}

resource "aws_rds_cluster_parameter_group" "trillian" {
  name        = "trillian-pg"
  family      = "aurora5.6"

  # Whether InnoDB returns errors rather than warnings for exceptional conditions.
  # replaces: `sql_mode = STRICT_ALL_TABLES`
  parameter {
    name  = "innodb_strict_mode"
    value = "1"
  }
}

/* The Instance */

/* select the latest official hvm amazon linux release */
data "aws_ami" "trillian" {
  most_recent      = true
  executable_users = ["all"]

  name_regex = "^amzn-ami-hvm"
  owners     = ["amazon"]
}

resource "aws_security_group" "trillian" {
  name        = "trillian"
  description = "Expose Rest, TPC and SSH endpoint to local cidr"

  ingress {
    from_port   = 8090
    to_port     = 8091
    protocol    = "tcp"
    cidr_blocks = ["${var.WHITELIST_CIDR}"]
  }
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["${var.WHITELIST_CIDR}"]
  }

  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    cidr_blocks     = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "trillian" {
  ami                         = "${data.aws_ami.trillian.id}"
  instance_type               = "t2.medium"
  vpc_security_group_ids      = ["${aws_security_group.trillian.id}"]
  associate_public_ip_address = true

  tags {
    Name = "trillian"
  }

  user_data =  <<EOF
#!/bin/bash

set -e

yum update -y
yum install -y git mysql

# install golang
curl -o /tmp/go.tar.gz https://storage.googleapis.com/golang/go1.8.3.linux-amd64.tar.gz
tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=$PATH:/usr/local/go/bin
mkdir -p /go
export GOPATH=/go

# Install Trillian
go get github.com/google/trillian/server/trillian_log_server

# Setup the DB
cd /go/src/github.com/google/trillian
export DB_USER=root
export DB_PASSWORD=${var.DB_PASSWORD}
export DB_HOST=${aws_rds_cluster.trillian.endpoint}
export DB_DATABASE=test
./scripts/resetdb.sh --verbose --force -h $DB_HOST

# Startup the Server
RPC_PORT=8090
HTTP_PORT=8091
/go/bin/trillian_log_server \
	--mysql_uri="${DB_USER}:${DB_PASSWORD}@tcp(${DB_HOST})/${DB_DATABASE}" \
	--rpc_endpoint="$HOST:$RPC_PORT" \
	--http_endpoint="$HOST:$HTTP_PORT" \
	--alsologtostderr
EOF

}
