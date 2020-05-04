ALTER TABLE routes
    MODIFY start_lat FLOAT(10, 3),
    MODIFY start_lon FLOAT(10, 3),
    MODIFY finish_lat  FLOAT(10, 3),
    MODIFY finish_lon FLOAT(10, 3),
    MODIFY length INT(11),
    MODIFY radius INT(11) NOT NULL DEFAULT 0;
