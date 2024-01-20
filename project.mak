# Extensions to the standard build process for the project.

# TODO update this if the root module gets packages
GO_MODULE_SLUGS_NO_PACKAGES = root
GO_MODULE_SLUGS_NO_UPDATE = sql.export.mysql
GO_MODULE_SLUGS_NO_BETTERALIGN = prompt
GRIT_SRC ?= https://github.com/joeycumines/go-utilpkg.git
GRIT_DST ?= \
    catrate$(MAP_SEPARATOR)https://github.com/joeycumines/go-catrate.git \
    fangrpcstream$(MAP_SEPARATOR)https://github.com/joeycumines/go-fangrpcstream.git \
    logiface$(MAP_SEPARATOR)https://github.com/joeycumines/logiface.git \
    logiface-logrus$(MAP_SEPARATOR)https://github.com/joeycumines/ilogrus.git \
    logiface-stumpy$(MAP_SEPARATOR)https://github.com/joeycumines/stumpy.git \
    logiface-testsuite$(MAP_SEPARATOR)https://github.com/joeycumines/logiface-testsuite.git \
    logiface-zerolog$(MAP_SEPARATOR)https://github.com/joeycumines/izerolog.git \
    longpoll$(MAP_SEPARATOR)https://github.com/joeycumines/go-longpoll.git \
    microbatch$(MAP_SEPARATOR)https://github.com/joeycumines/go-microbatch.git \
    smartpoll$(MAP_SEPARATOR)https://github.com/joeycumines/go-smartpoll.git \
    sql$(MAP_SEPARATOR)https://github.com/joeycumines/go-sql.git \
    prompt$(MAP_SEPARATOR)https://github.com/joeycumines/go-prompt.git
