import api from "./api.ts";
import "dotenv/config";
import logger from "./util/logger.ts";
import process from "node:process";

const server = api({ title: "@eyevinn/ad-normalizer" });

const PORT = process.env.PORT ? Number(process.env.PORT) : 8000;

server.listen({ port: PORT, host: "0.0.0.0" }, (err, address) => {
  if (err) {
    throw err;
  }
  logger.info(`Server listening on ${address}`);
});

export default server;
