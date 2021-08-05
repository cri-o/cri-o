pkg: pkg.overrideAttrs (x: {
  doCheck = false;
  configureFlags = (x.configureFlags or [ ]) ++ [
    "--without-shared"
    "--disable-shared"
  ];
  dontDisableStatic = true;
  enableSharedExecutables = false;
  enableStatic = true;
})
