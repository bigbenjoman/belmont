# Tech Plan

Plain CommonJS modules at the repo root, one file per operation. No build step,
no dependencies. Each module is required directly by `node --test`.
