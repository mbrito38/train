package trainCommand

import (
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/shaoshing/train"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const CompressorFileName = "yuicompressor-2.4.7.jar"

var Helps = `Available commands:
	 bundle: [default]
   upgrade: get and install latest train.
`

func Bundle(assetsPath string, publicPath string) {
	train.Config.AssetsPath = assetsPath
	train.Config.PublicPath = publicPath

	fmt.Println("bundle assets from: ", train.Config.AssetsPath)
	if !prepareEnv() {
		return
	}
	removeAssets()
	copyAssets()
	bundleAssets()
	compressAssets()
	fingerPrintAssets()
}

func prepareEnv() bool {
	public, err := os.Stat(train.Config.PublicPath)

	if err != nil && os.IsNotExist(err) {
		err := os.Mkdir(train.Config.PublicPath, os.FileMode(0777))
		if err != nil {
			panic(err)
		}
	} else if !public.IsDir() {
		fmt.Println("Can't create public directory automatically because a file with the same name already exists.\nPlease consider renaming your file or moving it to another folder.")
		return false
	}

	return true
}

func removeAssets() {
	fmt.Println("-> clean bundled assets")
	publicAssetPath := path.Join(train.Config.PublicPath, train.Config.AssetsUrl)
	_, err := os.Stat(publicAssetPath)
	if err != nil && os.IsNotExist(err) {
		return
	}
	if _, err := bash("rm -rf " + publicAssetPath); err != nil {
		panic(err)
	}
}

func copyAssets() {
	fmt.Println("-> copy assets from", train.Config.AssetsPath)

	publicAssetPath := path.Join(train.Config.PublicPath, train.Config.AssetsUrl)
	if _, err := bash("cp -rf " + train.Config.AssetsPath + " " + publicAssetPath); err != nil {
		panic(err)
	}
}

var mapCompiledExt = map[string]string{
	".sass":   ".css",
	".scss":   ".css",
	".coffee": ".js",
}

func bundleAssets() {
	fmt.Println("-> bundle and compile assets")

	train.Config.BundleAssets = true
	publicAssetPath := path.Join(train.Config.PublicPath + train.Config.AssetsUrl)

	filepath.Walk(publicAssetPath, func(filePath string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		assetUrl := strings.Replace(filePath, publicAssetPath, train.Config.AssetsUrl, 1)
		fileExt := path.Ext(filePath)
		switch fileExt {
		case ".js", ".css":
			if hasRequireDirectives(filePath) {
				content, err := train.ReadAsset(assetUrl)
				if err != nil {
					removeAssets()
					panic(err)
				}
				ioutil.WriteFile(filePath, []byte(content), os.ModeDevice)
			}
		case ".sass", ".scss", ".coffee":
			if path.Base(filePath)[0] == '_' {
				return nil
			}

			content, err := train.ReadAsset(assetUrl)
			if err != nil {
				removeAssets()
				fmt.Println("Error when reading asset: ", assetUrl)
				panic(err)
			}
			compiledPath := strings.Replace(filePath, fileExt, mapCompiledExt[fileExt], 1)
			os.Create(compiledPath)
			ioutil.WriteFile(compiledPath, []byte(content), os.ModeDevice)
		default:
			return nil
		}
		return nil
	})
}

func hasRequireDirectives(filePath string) bool {
	b_content, _ := ioutil.ReadFile(filePath)
	content := string(b_content)
	fileExt := path.Ext(filePath)
	header := train.FindDirectivesHeader(&content, fileExt)
	return len(header) != 0
}

func compressAssets() {
	fmt.Println("-> compress assets")

	jsFiles, cssFiles := getCompiledAssets(regexp.MustCompile(`(min\.\w+$)|\/min\/`))

	compress(jsFiles, ".js$:.js")
	compress(cssFiles, ".css$:.css")
}

func compress(files []string, option string) {
	if len(files) == 0 {
		return
	}

	_, err := exec.LookPath("java")
	if err != nil {
		fmt.Println("You don't have Java installed.")
		return
	}

	fmt.Println(files)

	_, filename, _, _ := runtime.Caller(1)
	pkgPath := path.Dir(filename)
	yuicompressor := pkgPath + "/" + CompressorFileName
	var out string
	if out, err = bash("java -jar " + yuicompressor + " -o '" + option + "' " + strings.Join(files, " ")); err != nil {
		fmt.Println("YUI Compressor error:", out)
		panic(err)
	}
}

func getCompiledAssets(filter *regexp.Regexp) (jsFiles []string, cssFiles []string) {
	publicAssetPath := train.Config.PublicPath + train.Config.AssetsUrl
	filepath.Walk(publicAssetPath, func(filePath string, info os.FileInfo, err error) error {
		fileExt := path.Ext(filePath)
		if filter != nil && filter.Match([]byte(filePath)) {
			return nil
		}
		switch fileExt {
		case ".js":
			jsFiles = append(jsFiles, filePath)
		case ".css":
			cssFiles = append(cssFiles, filePath)
		}
		return nil
	})

	return
}

func fingerPrintAssets() {
	fmt.Println("-> Fingerprinting Assets")

	assets, cssFiles := getCompiledAssets(nil)
	for _, file := range cssFiles {
		assets = append(assets, file)
	}

	fpAssets := train.FpAssets{}
	for _, asset := range assets {
		fpAsset, assetContent, err := GetHashedAsset(asset)
		if err != nil {
			return
		}

		err = ioutil.WriteFile(fpAsset, assetContent, 0644)
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}

		fpAssets[asset] = fpAsset
	}

	if err := train.WriteToManifest(fpAssets); err != nil {
		panic(err)
	}
}

func GetHashedAsset(assetPath string) (hashedPath string, content []byte, err error) {
	content, err = ioutil.ReadFile(assetPath)
	if err != nil {
		err = errors.New(fmt.Sprintf("Fingerprint Error: %s\n", err))
		return
	}

	h := md5.New()
	io.WriteString(h, string(content))
	fpStr := string(h.Sum(nil))

	dir, file := filepath.Split(assetPath)
	ext := filepath.Ext(file)
	filename := filepath.Base(file)
	filename = filename[0:strings.LastIndex(filename, ext)]

	hashedPath = fmt.Sprintf("%s%s-%x%s", dir, filename, fpStr, ext)
	return
}
