#!/bin/bash
echo "Completely wipe and reset database 'test'."
read -p "Are you sure? " -n 1 -r
if [[ $REPLY =~ ^[Yy]$ ]]
then
    # A command line supplied -u will override the first argument.
    mysql -u root "$@" -e 'DROP DATABASE IF EXISTS test;'
    mysql -u root "$@" -e 'CREATE DATABASE test;'
    mysql -u root "$@" -e "GRANT ALL ON test.* TO 'test'@'localhost' IDENTIFIED BY 'zaphod';"
    mysql -u root "$@" -D test < storage/sql/mysql/storage.sql
fi
echo
