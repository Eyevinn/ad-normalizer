import { createLogger, transports, format } from 'winston';

export default createLogger({
  level: process.env.LOG_LEVEL ? process.env.LOG_LEVEL.toLowerCase() : 'info',
  format: format.combine(format.timestamp(), format.json()),
  transports: [new transports.Console()]
});
