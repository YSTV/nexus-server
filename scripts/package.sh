arches=(amd64 386)

sudo apt-get install ruby ruby-dev rubygems gcc make
gem install --no-ri --no-rdoc fpm

for arch in ${arches[@]}; do
    fpm -a $arch -s dir -t deb -n nexus-server -p dist/ \
        --config-files /etc/nexus-server/nexus-server.conf -v `git describe --tags` \
        --post-install=scripts/post-install.sh \
        --post-uninstall=scripts/post-uninstall.sh \
        -m "`git for-each-ref --format '%(taggername) %(taggeremail)' refs/tags/$CIRCLE_TAG --count=1`" \
        dist/nexus-server_linux_$arch=/usr/bin  \
        nexus-server.conf.dist=/etc/nexus-server/nexus-server.conf
done
