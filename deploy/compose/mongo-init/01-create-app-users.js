const databaseName = process.env.MONGODB_DATABASE || process.env.MONGO_DB || "moogle";
const queryUsername = process.env.MONGODB_QUERY_USERNAME;
const queryPassword = process.env.MONGODB_QUERY_PASSWORD;
const pipelineUsername = process.env.MONGO_USERNAME;
const pipelinePassword = process.env.MONGO_PASSWORD;

if (!queryUsername || !queryPassword || !pipelineUsername || !pipelinePassword) {
  throw new Error("Mongo app user bootstrap requires query and pipeline credentials");
}

const appDb = db.getSiblingDB(databaseName);

appDb.createUser({
  user: queryUsername,
  pwd: queryPassword,
  roles: [{ role: "read", db: databaseName }],
});

appDb.createUser({
  user: pipelineUsername,
  pwd: pipelinePassword,
  roles: [{ role: "readWrite", db: databaseName }],
});
