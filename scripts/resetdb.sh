#!/bin/bash
echo "Completely wipe and reset database 'test'."
read -p "Are you sure? " -n 1 -r
if [[ $REPLY =~ ^[Yy]$ ]]
then
    # A command line supplied -u will override the first argument.
    mysql -u root "$@" -e 'DROP DATABASE IF EXISTS test;'
    mysql -u root "$@" -e 'CREATE DATABASE test;'
    mysql -u root "$@" -e "GRANT ALL ON test.* TO 'test'@'localhost' IDENTIFIED BY 'zaphod';"
    mysql -u root "$@" -D test < storage/mysql/storage.sql
    #TODO(codingllama): remove when db connection check doesn't ping log 0
    mysql -u root "$@" -D test -e "INSERT INTO Trees VALUES (0, 1, 'LOG', 'SHA256', 'SHA256', false)"
    mysql -u root "$@" -D test -e "INSERT INTO TreeControl VALUES(0, false, false, false, 1, 1)"
fi
echo
