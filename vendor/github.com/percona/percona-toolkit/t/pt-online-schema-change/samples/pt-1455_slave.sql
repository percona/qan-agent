STOP SLAVE;
CHANGE REPLICATION FILTER REPLICATE_IGNORE_DB = (`ignored_db`);
CHANGE REPLICATION FILTER REPLICATE_IGNORE_TABLE = (`idb1`.`it1`, `idb1`.`it2`);
CHANGE REPLICATION FILTER REPLICATE_WILD_IGNORE_TABLE = ('employees.eit%');
RESET SLAVE;
START SLAVE;
