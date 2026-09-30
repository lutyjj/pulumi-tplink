// Version comes from a release tag or local build, not the committed schema.
import { readFileSync, writeFileSync } from "node:fs";
const schema = JSON.parse(readFileSync(process.argv[2], "utf8"));
delete schema.version;
writeFileSync(process.argv[3], `${JSON.stringify(schema, null, 2)}\n`);
