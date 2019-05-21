# Postgres LogStorage

## Notes and Caveats
The current LogStorage part of the Postgres implementation was based off what
was already written for MySQL.  Thus, the two user-defined functions included in 
storage.sql.  MySQL doesn't kill a transaction when a duplicate is detected, but 
PostgreSQL does.  So, to preserve the workflow, I included the two functions 
which trap this error and allow the code to continue executing.  The only other
change I made was to fully translate the MySQL queries to PostgreSQL compatible ones
and tidy up some of the extant tree storage code.

storage_unsafe.sql really isn't unsafe, but I have pulled some of the safety
rails from the tables to improve performance.  It also works under the notion that
there will only be a single tree in a given database.  An improvement on this theme 
would be to add all layers below the trees table in their own separate schemas.  
This would further eliminate indexs and foreign key requirements, but it should
be left for those who require enhanced performance.  Storage.sql should be fine for most applications
