# Extensions to the standard build process for the project.

# TODO update this if the root module gets packages
GO_MODULE_SLUGS_NO_PACKAGES = root
GO_MODULE_SLUGS_NO_UPDATE = sql.export.mysql
GRIT_SRC ?= git@github.com:joeycumines/go-utilpkg.git
GRIT_DST ?= \
    logiface$(MAP_SEPARATOR)git@github.com:joeycumines/logiface.git \
    logiface-logrus$(MAP_SEPARATOR)git@github.com:joeycumines/ilogrus.git \
    logiface-stumpy$(MAP_SEPARATOR)git@github.com:joeycumines/stumpy.git \
    logiface-testsuite$(MAP_SEPARATOR)git@github.com:joeycumines/logiface-testsuite.git \
    logiface-zerolog$(MAP_SEPARATOR)git@github.com:joeycumines/izerolog.git \
    sql$(MAP_SEPARATOR)git@github.com:joeycumines/go-sql.git
