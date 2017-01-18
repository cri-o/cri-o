#!/usr/bin/env bash

PROJECT=github.com/kubernetes-incubator/cri-o

# Downloads dependencies into vendor/ directory
mkdir -p vendor

original_GOPATH=$GOPATH
export GOPATH="${PWD}/vendor:$GOPATH"

find="/usr/bin/find"

clone() {
	local delete_vendor=true
	if [ "x$1" = x--keep-vendor ]; then
		delete_vendor=false
		shift
	fi

	local vcs="$1"
	local pkg="$2"
	local rev="$3"
	local url="$4"

	: ${url:=https://$pkg}
	local target="vendor/$pkg"

	echo -n "$pkg @ $rev: "

	if [ -d "$target" ]; then
		echo -n 'rm old, '
		rm -rf "$target"
	fi

	echo -n 'clone, '
	case "$vcs" in
		git)
			git clone --quiet --no-checkout "$url" "$target"
			( cd "$target" && git checkout --quiet "$rev" && git reset --quiet --hard "$rev" -- )
			;;
		hg)
			hg clone --quiet --updaterev "$rev" "$url" "$target"
			;;
	esac

	echo -n 'rm VCS, '
	( cd "$target" && rm -rf .{git,hg} )

	if $delete_vendor; then
		echo -n 'rm vendor, '
		( cd "$target" && rm -rf vendor Godeps/_workspace )
	fi

	echo done
}

clean() {
	# If $GOPATH starts with ./vendor, (go list) shows the short-form import paths for packages inside ./vendor.
	# So, reset GOPATH to the external value (without ./vendor), so that the grep -v works.
	local packages=($(GOPATH=$original_GOPATH go list -e ./... | grep -v "^${PROJECT}/vendor"))
	local platforms=( linux/amd64 linux/386 )

	local buildTagSets=( seccomp )

	echo

	echo -n 'collecting import graph, '
	local IFS=$'\n'
	local imports=( $(
		for platform in "${platforms[@]}"; do
			for buildTags in "" "${buildTagSets[@]}"; do
				export GOOS="${platform%/*}";
				export GOARCH="${platform##*/}";
				go list -e -tags "$buildTags" -f '{{join .Deps "\n"}}' "${packages[@]}"
				go list -e -tags "$buildTags" -f '{{join .TestImports "\n"}}' "${packages[@]}"
			done
		done | grep -vE "^${PROJECT}" | sort -u
	) )
	# .TestImports does not include indirect dependencies, so do one more iteration.
	imports+=( $(
		go list -e -f '{{join .Deps "\n"}}' "${imports[@]}" | grep -vE "^${PROJECT}" | sort -u
	) )
	imports=( $(go list -e -f '{{if not .Standard}}{{.ImportPath}}{{end}}' "${imports[@]}") )
	unset IFS

	echo -n 'pruning unused packages, '
	findArgs=(
		# This directory contains only .c and .h files which are necessary
		# -path vendor/github.com/mattn/go-sqlite3/code
	)
	for import in "${imports[@]}"; do
		[ "${#findArgs[@]}" -eq 0 ] || findArgs+=( -or )
		findArgs+=( -path "vendor/$import" )
	done
	local IFS=$'\n'
	local prune=( $($find vendor -depth -type d -not '(' "${findArgs[@]}" ')') )
	unset IFS
	for dir in "${prune[@]}"; do
		$find "$dir" -maxdepth 1 -not -type d -not -name 'LICENSE*' -not -name 'COPYING*' -exec rm -v -f '{}' ';'
		rmdir "$dir" 2>/dev/null || true
	done

	echo -n 'pruning unused files, '
	$find vendor -type f -name '*_test.go' -exec rm -v '{}' ';'

	echo done
}

# Fix up hard-coded imports that refer to Godeps paths so they'll work with our vendoring
fix_rewritten_imports () {
       local pkg="$1"
       local remove="${pkg}/Godeps/_workspace/src/"
       local target="vendor/$pkg"

       echo "$pkg: fixing rewritten imports"
       $find "$target" -name \*.go -exec sed -i -e "s|\"${remove}|\"|g" {} \;
}
