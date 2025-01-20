import api from './api';
import 'dotenv/config';
import getConfiguration from './config/config';
import logger from './util/logger';

const server = api({ title: '@eyevinn/typescript-nodejs' });

const PORT = process.env.PORT ? Number(process.env.PORT) : 8000;

server.listen({ port: PORT, host: '0.0.0.0' }, (err, address) => {
  if (err) {
    throw err;
  }
  console.log(`Server listening on ${address}`);
});

export default server;
